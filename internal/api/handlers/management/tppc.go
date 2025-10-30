package management

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

// GetTppc returns the current Tppc configuration block.
func (h *Handler) GetTppc(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusOK, gin.H{"tppc": config.TppcConfig{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tppc": h.cfg.Tppc})
}

// PutTppc replaces the Tppc configuration block.
func (h *Handler) PutTppc(c *gin.Context) {
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	var body config.TppcConfig
	if err := json.Unmarshal(data, &body); err != nil {
		// also accept {"tppc": {...}}
		var wrapper struct {
			Tppc *config.TppcConfig `json:"tppc"`
		}
		if err2 := json.Unmarshal(data, &wrapper); err2 != nil || wrapper.Tppc == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		body = *wrapper.Tppc
	}
	normalizeTppc(&body)
	// validate using full config clone
	newCfg := *h.cfg
	newCfg.Tppc = body
	if err := config.ValidateTppc(&newCfg); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid_tppc", "message": err.Error()})
		return
	}
	h.cfg.Tppc = body
	h.persist(c)
}

// PatchTppc updates selected fields in the Tppc configuration block.
func (h *Handler) PatchTppc(c *gin.Context) {
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	// detect provided keys
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	if v, ok := raw["tppc"]; ok {
		if m, ok2 := v.(map[string]any); ok2 {
			raw = m
		}
	}
	var body config.TppcConfig
	_ = json.Unmarshal(data, &body) // best-effort struct mapping

	cur := h.cfg.Tppc

	// Handle providers array patch
	if providersRaw, ok := raw["providers"]; ok {
		if providersArray, ok2 := providersRaw.([]any); ok2 {
			// Update entire providers array
			if len(providersArray) == 0 {
				cur.Providers = []config.TppcProvider{}
			} else {
				// For now, replace the entire array (can be enhanced for partial updates)
				cur.Providers = body.Providers
			}
		}
	}

	// Read-only field must not be set by clients
	normalizeTppc(&cur)
	// validate using full config clone
	newCfg := *h.cfg
	newCfg.Tppc = cur
	if err := config.ValidateTppc(&newCfg); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid_tppc", "message": err.Error()})
		return
	}
	h.cfg.Tppc = cur
	h.persist(c)
}

// normalizeTppc normalizes Tppc configuration
func normalizeTppc(tc *config.TppcConfig) {
	if tc == nil {
		return
	}
	// Trim whitespace for all provider fields
	for i := range tc.Providers {
		tc.Providers[i].Name = strings.TrimSpace(tc.Providers[i].Name)
		tc.Providers[i].BaseURL = strings.TrimSpace(tc.Providers[i].BaseURL)
		tc.Providers[i].APIKey = strings.TrimSpace(tc.Providers[i].APIKey)
	}
}
