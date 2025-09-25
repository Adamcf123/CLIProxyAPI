package geminiwebapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/auth/gemini"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/translator/translator"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	bolt "go.etcd.io/bbolt"
)

const (
	geminiWebDefaultTimeoutSec = 300
)

type GeminiWebState struct {
	cfg         *config.Config
	token       *gemini.GeminiWebTokenStorage
	storagePath string

	stableClientID string
	accountID      string

	reqMu  sync.Mutex
	client *GeminiClient

	tokenMu    sync.Mutex
	tokenDirty bool

	convMu    sync.RWMutex
	convStore map[string][]string
	convData  map[string]ConversationRecord
	convIndex map[string]string

	lastRefresh time.Time
}

func NewGeminiWebState(cfg *config.Config, token *gemini.GeminiWebTokenStorage, storagePath string) *GeminiWebState {
	state := &GeminiWebState{
		cfg:         cfg,
		token:       token,
		storagePath: storagePath,
		convStore:   make(map[string][]string),
		convData:    make(map[string]ConversationRecord),
		convIndex:   make(map[string]string),
	}
	suffix := Sha256Hex(token.Secure1PSID)
	if len(suffix) > 16 {
		suffix = suffix[:16]
	}
	state.stableClientID = "gemini-web-" + suffix
	if storagePath != "" {
		base := strings.TrimSuffix(filepath.Base(storagePath), filepath.Ext(storagePath))
		if base != "" {
			state.accountID = base
		} else {
			state.accountID = suffix
		}
	} else {
		state.accountID = suffix
	}
	state.loadConversationCaches()
	return state
}

func (s *GeminiWebState) loadConversationCaches() {
	if path := s.convStorePath(); path != "" {
		if store, err := LoadConvStore(path); err == nil {
			s.convStore = store
		}
	}
	if path := s.convDataPath(); path != "" {
		if items, index, err := LoadConvData(path); err == nil {
			s.convData = items
			s.convIndex = index
		}
	}
}

func (s *GeminiWebState) convStorePath() string {
	base := s.storagePath
	if base == "" {
		base = s.accountID + ".json"
	}
	return ConvStorePath(base)
}

func (s *GeminiWebState) convDataPath() string {
	base := s.storagePath
	if base == "" {
		base = s.accountID + ".json"
	}
	return ConvDataPath(base)
}

func (s *GeminiWebState) GetRequestMutex() *sync.Mutex { return &s.reqMu }

func (s *GeminiWebState) EnsureClient() error {
	if s.client != nil && s.client.Running {
		return nil
	}
	proxyURL := ""
	if s.cfg != nil {
		proxyURL = s.cfg.ProxyURL
	}
	s.client = NewGeminiClient(
		s.token.Secure1PSID,
		s.token.Secure1PSIDTS,
		proxyURL,
	)
	timeout := geminiWebDefaultTimeoutSec
	if err := s.client.Init(float64(timeout), false); err != nil {
		s.client = nil
		return err
	}
	s.lastRefresh = time.Now()
	return nil
}

func (s *GeminiWebState) Refresh(ctx context.Context) error {
	_ = ctx
	proxyURL := ""
	if s.cfg != nil {
		proxyURL = s.cfg.ProxyURL
	}
	s.client = NewGeminiClient(
		s.token.Secure1PSID,
		s.token.Secure1PSIDTS,
		proxyURL,
	)
	timeout := geminiWebDefaultTimeoutSec
	if err := s.client.Init(float64(timeout), false); err != nil {
		return err
	}
	// Attempt rotation proactively to persist new TS sooner
	if newTS, err := s.client.RotateTS(); err == nil && newTS != "" && newTS != s.token.Secure1PSIDTS {
		s.tokenMu.Lock()
		s.token.Secure1PSIDTS = newTS
		s.tokenDirty = true
		if s.client != nil && s.client.Cookies != nil {
			s.client.Cookies["__Secure-1PSIDTS"] = newTS
		}
		s.tokenMu.Unlock()
	}
	s.lastRefresh = time.Now()
	return nil
}

func (s *GeminiWebState) TokenSnapshot() *gemini.GeminiWebTokenStorage {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	c := *s.token
	return &c
}

type geminiWebPrepared struct {
	handlerType   string
	translatedRaw []byte
	prompt        string
	uploaded      []string
	chat          *ChatSession
	cleaned       []RoleText
	underlying    string
	reuse         bool
	tagged        bool
	originalRaw   []byte
}

func (s *GeminiWebState) prepare(ctx context.Context, modelName string, rawJSON []byte, stream bool, original []byte) (*geminiWebPrepared, *interfaces.ErrorMessage) {
	res := &geminiWebPrepared{originalRaw: original}
	res.translatedRaw = bytes.Clone(rawJSON)
	if handler, ok := ctx.Value("handler").(interfaces.APIHandler); ok && handler != nil {
		res.handlerType = handler.HandlerType()
		res.translatedRaw = translator.Request(res.handlerType, constant.GeminiWeb, modelName, res.translatedRaw, stream)
	}
	recordAPIRequest(ctx, s.cfg, res.translatedRaw)

	messages, files, mimes, msgFileIdx, err := ParseMessagesAndFiles(res.translatedRaw)
	if err != nil {
		return nil, &interfaces.ErrorMessage{StatusCode: 400, Error: fmt.Errorf("bad request: %w", err)}
	}
	cleaned := SanitizeAssistantMessages(messages)
	res.cleaned = cleaned
	res.underlying = MapAliasToUnderlying(modelName)
	model, err := ModelFromName(res.underlying)
	if err != nil {
		return nil, &interfaces.ErrorMessage{StatusCode: 400, Error: err}
	}

	var meta []string
	useMsgs := cleaned
	filesSubset := files
	mimesSubset := mimes

	if s.useReusableContext() {
		reuseMeta, remaining := s.findReusableSession(res.underlying, cleaned)
		if len(reuseMeta) > 0 {
			res.reuse = true
			meta = reuseMeta
			if len(remaining) == 1 {
				useMsgs = []RoleText{remaining[0]}
			} else if len(remaining) > 1 {
				useMsgs = remaining
			} else if len(cleaned) > 0 {
				useMsgs = []RoleText{cleaned[len(cleaned)-1]}
			}
			if len(useMsgs) == 1 && len(messages) > 0 && len(msgFileIdx) == len(messages) {
				lastIdx := len(msgFileIdx) - 1
				idxs := msgFileIdx[lastIdx]
				if len(idxs) > 0 {
					filesSubset = make([][]byte, 0, len(idxs))
					mimesSubset = make([]string, 0, len(idxs))
					for _, fi := range idxs {
						if fi >= 0 && fi < len(files) {
							filesSubset = append(filesSubset, files[fi])
							if fi < len(mimes) {
								mimesSubset = append(mimesSubset, mimes[fi])
							} else {
								mimesSubset = append(mimesSubset, "")
							}
						}
					}
				} else {
					filesSubset = nil
					mimesSubset = nil
				}
			} else {
				filesSubset = nil
				mimesSubset = nil
			}
		} else {
			if len(cleaned) >= 2 && strings.EqualFold(cleaned[len(cleaned)-2].Role, "assistant") {
				keyUnderlying := AccountMetaKey(s.accountID, res.underlying)
				keyAlias := AccountMetaKey(s.accountID, modelName)
				s.convMu.RLock()
				fallbackMeta := s.convStore[keyUnderlying]
				if len(fallbackMeta) == 0 {
					fallbackMeta = s.convStore[keyAlias]
				}
				s.convMu.RUnlock()
				if len(fallbackMeta) > 0 {
					meta = fallbackMeta
					useMsgs = []RoleText{cleaned[len(cleaned)-1]}
					res.reuse = true
					filesSubset = nil
					mimesSubset = nil
				}
			}
		}
	} else {
		keyUnderlying := AccountMetaKey(s.accountID, res.underlying)
		keyAlias := AccountMetaKey(s.accountID, modelName)
		s.convMu.RLock()
		if v, ok := s.convStore[keyUnderlying]; ok && len(v) > 0 {
			meta = v
		} else {
			meta = s.convStore[keyAlias]
		}
		s.convMu.RUnlock()
	}

	res.tagged = NeedRoleTags(useMsgs)
	if res.reuse && len(useMsgs) == 1 {
		res.tagged = false
	}

	enableXML := s.cfg != nil && s.cfg.GeminiWeb.CodeMode
	useMsgs = AppendXMLWrapHintIfNeeded(useMsgs, !enableXML)

	res.prompt = BuildPrompt(useMsgs, res.tagged, res.tagged)
	if strings.TrimSpace(res.prompt) == "" {
		return nil, &interfaces.ErrorMessage{StatusCode: 400, Error: errors.New("bad request: empty prompt after filtering system/thought content")}
	}

	uploaded, upErr := MaterializeInlineFiles(filesSubset, mimesSubset)
	if upErr != nil {
		return nil, upErr
	}
	res.uploaded = uploaded

	if err = s.EnsureClient(); err != nil {
		return nil, &interfaces.ErrorMessage{StatusCode: 500, Error: err}
	}
	chat := s.client.StartChat(model, s.getConfiguredGem(), meta)
	chat.SetRequestedModel(modelName)
	res.chat = chat

	return res, nil
}

func (s *GeminiWebState) Send(ctx context.Context, modelName string, reqPayload []byte, opts cliproxyexecutor.Options) ([]byte, *interfaces.ErrorMessage, *geminiWebPrepared) {
	prep, errMsg := s.prepare(ctx, modelName, reqPayload, opts.Stream, opts.OriginalRequest)
	if errMsg != nil {
		return nil, errMsg, nil
	}
	defer CleanupFiles(prep.uploaded)

	output, err := SendWithSplit(prep.chat, prep.prompt, prep.uploaded, s.cfg)
	if err != nil {
		return nil, s.wrapSendError(err), nil
	}

	// Hook: For gemini-2.5-flash-image-preview, if the API returns only images without any text,
	// inject a small textual summary so that conversation persistence has non-empty assistant text.
	// This helps conversation recovery (conv store) to match sessions reliably.
	if strings.EqualFold(modelName, "gemini-2.5-flash-image-preview") {
		if len(output.Candidates) > 0 {
			c := output.Candidates[output.Chosen]
			hasNoText := strings.TrimSpace(c.Text) == ""
			hasImages := len(c.GeneratedImages) > 0 || len(c.WebImages) > 0
			if hasNoText && hasImages {
				// Build a stable, concise fallback text. Avoid dynamic details to keep hashes stable.
				// Prefer a deterministic phrase with count to aid users while keeping consistency.
				fallback := "Done"
				// Mutate the chosen candidate's text so both response conversion and
				// conversation persistence observe the same fallback.
				output.Candidates[output.Chosen].Text = fallback
			}
		}
	}

	gemBytes, err := ConvertOutputToGemini(&output, modelName, prep.prompt)
	if err != nil {
		return nil, &interfaces.ErrorMessage{StatusCode: 500, Error: err}, nil
	}

	s.addAPIResponseData(ctx, gemBytes)
	s.persistConversation(modelName, prep, &output)
	return gemBytes, nil, prep
}

func (s *GeminiWebState) wrapSendError(genErr error) *interfaces.ErrorMessage {
	status := 500
	var usage *UsageLimitExceeded
	var blocked *TemporarilyBlocked
	var invalid *ModelInvalid
	var valueErr *ValueError
	var timeout *TimeoutError
	switch {
	case errors.As(genErr, &usage):
		status = 429
	case errors.As(genErr, &blocked):
		status = 429
	case errors.As(genErr, &invalid):
		status = 400
	case errors.As(genErr, &valueErr):
		status = 400
	case errors.As(genErr, &timeout):
		status = 504
	}
	return &interfaces.ErrorMessage{StatusCode: status, Error: genErr}
}

func (s *GeminiWebState) persistConversation(modelName string, prep *geminiWebPrepared, output *ModelOutput) {
	if output == nil || prep == nil || prep.chat == nil {
		return
	}
	metadata := prep.chat.Metadata()
	if len(metadata) > 0 {
		keyUnderlying := AccountMetaKey(s.accountID, prep.underlying)
		keyAlias := AccountMetaKey(s.accountID, modelName)
		s.convMu.Lock()
		s.convStore[keyUnderlying] = metadata
		s.convStore[keyAlias] = metadata
		storeSnapshot := make(map[string][]string, len(s.convStore))
		for k, v := range s.convStore {
			if v == nil {
				continue
			}
			cp := make([]string, len(v))
			copy(cp, v)
			storeSnapshot[k] = cp
		}
		s.convMu.Unlock()
		_ = SaveConvStore(s.convStorePath(), storeSnapshot)
	}

	if !s.useReusableContext() {
		return
	}
	rec, ok := BuildConversationRecord(prep.underlying, s.stableClientID, prep.cleaned, output, metadata)
	if !ok {
		return
	}
	stableHash := HashConversation(rec.ClientID, prep.underlying, rec.Messages)
	accountHash := HashConversation(s.accountID, prep.underlying, rec.Messages)

	s.convMu.Lock()
	s.convData[stableHash] = rec
	s.convIndex["hash:"+stableHash] = stableHash
	if accountHash != stableHash {
		s.convIndex["hash:"+accountHash] = stableHash
	}
	dataSnapshot := make(map[string]ConversationRecord, len(s.convData))
	for k, v := range s.convData {
		dataSnapshot[k] = v
	}
	indexSnapshot := make(map[string]string, len(s.convIndex))
	for k, v := range s.convIndex {
		indexSnapshot[k] = v
	}
	s.convMu.Unlock()
	_ = SaveConvData(s.convDataPath(), dataSnapshot, indexSnapshot)
}

func (s *GeminiWebState) addAPIResponseData(ctx context.Context, line []byte) {
	appendAPIResponseChunk(ctx, s.cfg, line)
}

func (s *GeminiWebState) ConvertToTarget(ctx context.Context, modelName string, prep *geminiWebPrepared, gemBytes []byte) []byte {
	if prep == nil || prep.handlerType == "" {
		return gemBytes
	}
	if !translator.NeedConvert(prep.handlerType, constant.GeminiWeb) {
		return gemBytes
	}
	var param any
	out := translator.ResponseNonStream(prep.handlerType, constant.GeminiWeb, ctx, modelName, prep.originalRaw, prep.translatedRaw, gemBytes, &param)
	if prep.handlerType == constant.OpenAI && out != "" {
		newID := fmt.Sprintf("chatcmpl-%x", time.Now().UnixNano())
		if v := gjson.Parse(out).Get("id"); v.Exists() {
			out, _ = sjson.Set(out, "id", newID)
		}
	}
	return []byte(out)
}

func (s *GeminiWebState) ConvertStream(ctx context.Context, modelName string, prep *geminiWebPrepared, gemBytes []byte) []string {
	if prep == nil || prep.handlerType == "" {
		return []string{string(gemBytes)}
	}
	if !translator.NeedConvert(prep.handlerType, constant.GeminiWeb) {
		return []string{string(gemBytes)}
	}
	var param any
	return translator.Response(prep.handlerType, constant.GeminiWeb, ctx, modelName, prep.originalRaw, prep.translatedRaw, gemBytes, &param)
}

func (s *GeminiWebState) DoneStream(ctx context.Context, modelName string, prep *geminiWebPrepared) []string {
	if prep == nil || prep.handlerType == "" {
		return nil
	}
	if !translator.NeedConvert(prep.handlerType, constant.GeminiWeb) {
		return nil
	}
	var param any
	return translator.Response(prep.handlerType, constant.GeminiWeb, ctx, modelName, prep.originalRaw, prep.translatedRaw, []byte("[DONE]"), &param)
}

func (s *GeminiWebState) useReusableContext() bool {
	if s.cfg == nil {
		return true
	}
	return s.cfg.GeminiWeb.Context
}

func (s *GeminiWebState) findReusableSession(modelName string, msgs []RoleText) ([]string, []RoleText) {
	s.convMu.RLock()
	items := s.convData
	index := s.convIndex
	s.convMu.RUnlock()
	return FindReusableSessionIn(items, index, s.stableClientID, s.accountID, modelName, msgs)
}

func (s *GeminiWebState) getConfiguredGem() *Gem {
	if s.cfg != nil && s.cfg.GeminiWeb.CodeMode {
		return &Gem{ID: "coding-partner", Name: "Coding partner", Predefined: true}
	}
	return nil
}

// recordAPIRequest stores the upstream request payload in Gin context for request logging.
func recordAPIRequest(ctx context.Context, cfg *config.Config, payload []byte) {
	if cfg == nil || !cfg.RequestLog || len(payload) == 0 {
		return
	}
	if ginCtx, ok := ctx.Value("gin").(*gin.Context); ok && ginCtx != nil {
		ginCtx.Set("API_REQUEST", bytes.Clone(payload))
	}
}

// appendAPIResponseChunk appends an upstream response chunk to Gin context for request logging.
func appendAPIResponseChunk(ctx context.Context, cfg *config.Config, chunk []byte) {
	if cfg == nil || !cfg.RequestLog {
		return
	}
	data := bytes.TrimSpace(bytes.Clone(chunk))
	if len(data) == 0 {
		return
	}
	if ginCtx, ok := ctx.Value("gin").(*gin.Context); ok && ginCtx != nil {
		if existing, exists := ginCtx.Get("API_RESPONSE"); exists {
			if prev, okBytes := existing.([]byte); okBytes {
				prev = append(prev, data...)
				prev = append(prev, []byte("\n\n")...)
				ginCtx.Set("API_RESPONSE", prev)
				return
			}
		}
		ginCtx.Set("API_RESPONSE", data)
	}
}

// Persistence helpers --------------------------------------------------

// Sha256Hex computes the SHA256 hash of a string and returns its hex representation.
func Sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func ToStoredMessages(msgs []RoleText) []StoredMessage {
	out := make([]StoredMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, StoredMessage{
			Role:    m.Role,
			Content: m.Text,
		})
	}
	return out
}

func HashMessage(m StoredMessage) string {
	s := fmt.Sprintf(`{"content":%q,"role":%q}`, m.Content, strings.ToLower(m.Role))
	return Sha256Hex(s)
}

func HashConversation(clientID, model string, msgs []StoredMessage) string {
	var b strings.Builder
	b.WriteString(clientID)
	b.WriteString("|")
	b.WriteString(model)
	for _, m := range msgs {
		b.WriteString("|")
		b.WriteString(HashMessage(m))
	}
	return Sha256Hex(b.String())
}

// ConvStorePath returns the path for account-level metadata persistence based on token file path.
func ConvStorePath(tokenFilePath string) string {
	wd, err := os.Getwd()
	if err != nil || wd == "" {
		wd = "."
	}
	convDir := filepath.Join(wd, "conv")
	base := strings.TrimSuffix(filepath.Base(tokenFilePath), filepath.Ext(tokenFilePath))
	return filepath.Join(convDir, base+".bolt")
}

// ConvDataPath returns the path for full conversation persistence based on token file path.
func ConvDataPath(tokenFilePath string) string {
	wd, err := os.Getwd()
	if err != nil || wd == "" {
		wd = "."
	}
	convDir := filepath.Join(wd, "conv")
	base := strings.TrimSuffix(filepath.Base(tokenFilePath), filepath.Ext(tokenFilePath))
	return filepath.Join(convDir, base+".bolt")
}

// LoadConvStore reads the account-level metadata store from disk.
func LoadConvStore(path string) (map[string][]string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = db.Close()
	}()
	out := map[string][]string{}
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("account_meta"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var arr []string
			if len(v) > 0 {
				if e := json.Unmarshal(v, &arr); e != nil {
					// Skip malformed entries instead of failing the whole load
					return nil
				}
			}
			out[string(k)] = arr
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SaveConvStore writes the account-level metadata store to disk atomically.
func SaveConvStore(path string, data map[string][]string) error {
	if data == nil {
		data = map[string][]string{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()
	return db.Update(func(tx *bolt.Tx) error {
		// Recreate bucket to reflect the given snapshot exactly.
		if b := tx.Bucket([]byte("account_meta")); b != nil {
			if err = tx.DeleteBucket([]byte("account_meta")); err != nil {
				return err
			}
		}
		b, errCreateBucket := tx.CreateBucket([]byte("account_meta"))
		if errCreateBucket != nil {
			return errCreateBucket
		}
		for k, v := range data {
			enc, e := json.Marshal(v)
			if e != nil {
				return e
			}
			if e = b.Put([]byte(k), enc); e != nil {
				return e
			}
		}
		return nil
	})
}

// AccountMetaKey builds the key for account-level metadata map.
func AccountMetaKey(email, modelName string) string {
	return fmt.Sprintf("account-meta|%s|%s", email, modelName)
}

// LoadConvData reads the full conversation data and index from disk.
func LoadConvData(path string) (map[string]ConversationRecord, map[string]string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		_ = db.Close()
	}()
	items := map[string]ConversationRecord{}
	index := map[string]string{}
	err = db.View(func(tx *bolt.Tx) error {
		// Load conv_items
		if b := tx.Bucket([]byte("conv_items")); b != nil {
			if e := b.ForEach(func(k, v []byte) error {
				var rec ConversationRecord
				if len(v) > 0 {
					if e2 := json.Unmarshal(v, &rec); e2 != nil {
						// Skip malformed
						return nil
					}
					items[string(k)] = rec
				}
				return nil
			}); e != nil {
				return e
			}
		}
		// Load conv_index
		if b := tx.Bucket([]byte("conv_index")); b != nil {
			if e := b.ForEach(func(k, v []byte) error {
				index[string(k)] = string(v)
				return nil
			}); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return items, index, nil
}

// SaveConvData writes the full conversation data and index to disk atomically.
func SaveConvData(path string, items map[string]ConversationRecord, index map[string]string) error {
	if items == nil {
		items = map[string]ConversationRecord{}
	}
	if index == nil {
		index = map[string]string{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()
	return db.Update(func(tx *bolt.Tx) error {
		// Recreate items bucket
		if b := tx.Bucket([]byte("conv_items")); b != nil {
			if err = tx.DeleteBucket([]byte("conv_items")); err != nil {
				return err
			}
		}
		bi, errCreateBucket := tx.CreateBucket([]byte("conv_items"))
		if errCreateBucket != nil {
			return errCreateBucket
		}
		for k, rec := range items {
			enc, e := json.Marshal(rec)
			if e != nil {
				return e
			}
			if e = bi.Put([]byte(k), enc); e != nil {
				return e
			}
		}

		// Recreate index bucket
		if b := tx.Bucket([]byte("conv_index")); b != nil {
			if err = tx.DeleteBucket([]byte("conv_index")); err != nil {
				return err
			}
		}
		bx, errCreateBucket := tx.CreateBucket([]byte("conv_index"))
		if errCreateBucket != nil {
			return errCreateBucket
		}
		for k, v := range index {
			if e := bx.Put([]byte(k), []byte(v)); e != nil {
				return e
			}
		}
		return nil
	})
}

// BuildConversationRecord constructs a ConversationRecord from history and the latest output.
// Returns false when output is empty or has no candidates.
func BuildConversationRecord(model, clientID string, history []RoleText, output *ModelOutput, metadata []string) (ConversationRecord, bool) {
	if output == nil || len(output.Candidates) == 0 {
		return ConversationRecord{}, false
	}
	text := ""
	if t := output.Candidates[0].Text; t != "" {
		text = RemoveThinkTags(t)
	}
	final := append([]RoleText{}, history...)
	final = append(final, RoleText{Role: "assistant", Text: text})
	rec := ConversationRecord{
		Model:     model,
		ClientID:  clientID,
		Metadata:  metadata,
		Messages:  ToStoredMessages(final),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return rec, true
}

// FindByMessageListIn looks up a conversation record by hashed message list.
// It attempts both the stable client ID and a legacy email-based ID.
func FindByMessageListIn(items map[string]ConversationRecord, index map[string]string, stableClientID, email, model string, msgs []RoleText) (ConversationRecord, bool) {
	stored := ToStoredMessages(msgs)
	stableHash := HashConversation(stableClientID, model, stored)
	fallbackHash := HashConversation(email, model, stored)

	// Try stable hash via index indirection first
	if key, ok := index["hash:"+stableHash]; ok {
		if rec, ok2 := items[key]; ok2 {
			return rec, true
		}
	}
	if rec, ok := items[stableHash]; ok {
		return rec, true
	}
	// Fallback to legacy hash (email-based)
	if key, ok := index["hash:"+fallbackHash]; ok {
		if rec, ok2 := items[key]; ok2 {
			return rec, true
		}
	}
	if rec, ok := items[fallbackHash]; ok {
		return rec, true
	}
	return ConversationRecord{}, false
}

// FindConversationIn tries exact then sanitized assistant messages.
func FindConversationIn(items map[string]ConversationRecord, index map[string]string, stableClientID, email, model string, msgs []RoleText) (ConversationRecord, bool) {
	if len(msgs) == 0 {
		return ConversationRecord{}, false
	}
	if rec, ok := FindByMessageListIn(items, index, stableClientID, email, model, msgs); ok {
		return rec, true
	}
	if rec, ok := FindByMessageListIn(items, index, stableClientID, email, model, SanitizeAssistantMessages(msgs)); ok {
		return rec, true
	}
	return ConversationRecord{}, false
}

// FindReusableSessionIn returns reusable metadata and the remaining message suffix.
func FindReusableSessionIn(items map[string]ConversationRecord, index map[string]string, stableClientID, email, model string, msgs []RoleText) ([]string, []RoleText) {
	if len(msgs) < 2 {
		return nil, nil
	}
	searchEnd := len(msgs)
	for searchEnd >= 2 {
		sub := msgs[:searchEnd]
		tail := sub[len(sub)-1]
		if strings.EqualFold(tail.Role, "assistant") || strings.EqualFold(tail.Role, "system") {
			if rec, ok := FindConversationIn(items, index, stableClientID, email, model, sub); ok {
				remain := msgs[searchEnd:]
				return rec.Metadata, remain
			}
		}
		searchEnd--
	}
	return nil, nil
}
