package openai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestOpenAIStreamingTerminalErrorSetsStatusWhenNoChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	h := NewOpenAIAPIHandler(&handlers.BaseAPIHandler{Cfg: &config.SDKConfig{}})

	data := make(chan []byte)
	errs := make(chan *interfaces.ErrorMessage, 1)
	errs <- &interfaces.ErrorMessage{StatusCode: http.StatusUnauthorized, Error: errors.New("unauthorized")}
	close(errs)

	// No chunks are written before the terminal error, so headers are not committed yet.
	h.handleStreamResultWithPrefetched(c, w, func(error) {}, data, errs, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}
