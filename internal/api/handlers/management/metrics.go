package management

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type metricsFilters struct {
	Provider  *string `json:"provider"`
	Model     *string `json:"model"`
	Streaming *bool   `json:"streaming"`
}

type metricsMeta struct {
	Mode          string         `json:"mode"`
	RequestedFrom *string        `json:"requested_from"`
	RequestedTo   *string        `json:"requested_to"`
	EffectiveFrom string         `json:"effective_from"`
	EffectiveTo   string         `json:"effective_to"`
	Filters       metricsFilters `json:"filters"`
}

type metricsEnvelope[T any] struct {
	Meta  metricsMeta `json:"meta"`
	Data  T           `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

type metricsRow struct {
	RequestID    string   `json:"request_id"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	Streaming    bool     `json:"streaming"`
	StatusCode   *int64   `json:"status_code"`
	ErrorInfo    *string  `json:"error_info"`
	CreatedAt    string   `json:"created_at"`
	TPS          *float64 `json:"tps"`
	TTFTMillis   *int64   `json:"ttft_ms"`
	TPOTMillis   *int64   `json:"tpot_ms"`
	DurationMS   *int64   `json:"duration_ms"`
	InputTokens  *int64   `json:"input_tokens"`
	OutputTokens *int64   `json:"output_tokens"`
	TotalTokens  *int64   `json:"total_tokens"`
}

func (h *Handler) GetMetrics(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler unavailable"})
		return
	}

	requestID := strings.TrimSpace(c.Query("request_id"))
	provider := strings.TrimSpace(c.Query("provider"))
	model := strings.TrimSpace(c.Query("model"))
	streamingRaw := strings.TrimSpace(c.Query("streaming"))

	filters, err := parseMetricsFilters(provider, model, streamingRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, metricsEnvelope[any]{
			Error: err.Error(),
			Meta:  metricsMeta{Filters: filters},
		})
		return
	}

	// request_id branch: ignore mode/from/to/bucket, but still apply fail-fast validation
	// for shared filters (provider/model/streaming).
	if requestID != "" {
		_, _, effectiveFrom, effectiveTo, _ := parseMetricsTimeRange(h.nowUTC, "", "")
		meta := metricsMeta{
			Mode:          "request_id",
			RequestedFrom: nil,
			RequestedTo:   nil,
			EffectiveFrom: effectiveFrom.Format(time.RFC3339),
			EffectiveTo:   effectiveTo.Format(time.RFC3339),
			Filters:       filters,
		}
		row, err := h.queryMetricsByRequestID(c.Request.Context(), requestID)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, metricsEnvelope[any]{
					Error: "metrics not found",
					Meta:  meta,
				})
				return
			}
			c.JSON(http.StatusInternalServerError, metricsEnvelope[any]{
				Error: "failed to query metrics",
				Meta:  meta,
			})
			return
		}
		c.JSON(http.StatusOK, metricsEnvelope[metricsRow]{
			Meta: meta,
			Data: row,
		})
		return
	}

	mode := strings.TrimSpace(c.Query("mode"))
	if mode == "" {
		c.JSON(http.StatusBadRequest, metricsEnvelope[any]{
			Error: "mode is required",
			Meta: metricsMeta{
				Mode:    mode,
				Filters: filters,
			},
		})
		return
	}
	if mode != "percentiles" && mode != "buckets" {
		c.JSON(http.StatusBadRequest, metricsEnvelope[any]{
			Error: "invalid mode",
			Meta: metricsMeta{
				Mode:    mode,
				Filters: filters,
			},
		})
		return
	}

	fromRaw := strings.TrimSpace(c.Query("from"))
	toRaw := strings.TrimSpace(c.Query("to"))
	requestedFrom, requestedTo, effectiveFrom, effectiveTo, err := parseMetricsTimeRange(h.nowUTC, fromRaw, toRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, metricsEnvelope[any]{
			Error: err.Error(),
			Meta: metricsMeta{
				Mode:    mode,
				Filters: filters,
			},
		})
		return
	}

	meta := metricsMeta{
		Mode:          mode,
		RequestedFrom: requestedFrom,
		RequestedTo:   requestedTo,
		EffectiveFrom: effectiveFrom.Format(time.RFC3339),
		EffectiveTo:   effectiveTo.Format(time.RFC3339),
		Filters:       filters,
	}

	// Aggregation modes are implemented in subsequent plans.
	c.JSON(http.StatusNotImplemented, metricsEnvelope[any]{
		Error: "mode not implemented",
		Meta:  meta,
	})
}

func parseMetricsFilters(provider, model, streamingRaw string) (metricsFilters, error) {
	var out metricsFilters
	if provider != "" {
		p := provider
		out.Provider = &p
	}
	if model != "" {
		m := model
		out.Model = &m
	}
	if streamingRaw == "" {
		return out, nil
	}
	b, ok := parseFlexibleBool(streamingRaw)
	if !ok {
		return out, errBadRequest("invalid streaming")
	}
	out.Streaming = &b
	return out, nil
}

func parseFlexibleBool(v string) (bool, bool) {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

func parseMetricsTimeRange(nowUTC func() time.Time, fromRaw, toRaw string) (*string, *string, time.Time, time.Time, error) {
	if nowUTC == nil {
		nowUTC = func() time.Time { return time.Now().UTC() }
	}
	if (fromRaw == "") != (toRaw == "") {
		return nil, nil, time.Time{}, time.Time{}, errBadRequest("from and to must be provided together")
	}
	if fromRaw == "" && toRaw == "" {
		to := nowUTC().UTC()
		from := to.Add(-1 * time.Hour)
		return nil, nil, from, to, nil
	}
	fromT, err := time.Parse(time.RFC3339, fromRaw)
	if err != nil {
		return nil, nil, time.Time{}, time.Time{}, errBadRequest("invalid from")
	}
	toT, err := time.Parse(time.RFC3339, toRaw)
	if err != nil {
		return nil, nil, time.Time{}, time.Time{}, errBadRequest("invalid to")
	}
	fromT = fromT.UTC()
	toT = toT.UTC()
	if !fromT.Before(toT) {
		return nil, nil, time.Time{}, time.Time{}, errBadRequest("from must be before to")
	}
	fromStr := fromT.Format(time.RFC3339)
	toStr := toT.Format(time.RFC3339)
	return &fromStr, &toStr, fromT, toT, nil
}

type errBadRequest string

func (e errBadRequest) Error() string { return string(e) }

func (h *Handler) queryMetricsByRequestID(ctx context.Context, requestID string) (metricsRow, error) {
	db, err := h.openMetricsReadDB()
	if err != nil {
		return metricsRow{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const q = `SELECT
		request_id,
		provider,
		model,
		streaming,
		tps,
		ttft,
		tpot,
		input_tokens,
		output_tokens,
		total_tokens,
		duration_ms,
		status_code,
		error_info,
		created_at
	FROM metrics
	WHERE request_id = ?;`

	var (
		requestIDVal string
		providerVal  string
		modelVal     string
		streamingVal int64
		tpsVal       sql.NullFloat64
		ttftVal      sql.NullFloat64
		tpotVal      sql.NullFloat64
		inTok        sql.NullInt64
		outTok       sql.NullInt64
		totalTok     sql.NullInt64
		durMS        sql.NullInt64
		statusCode   sql.NullInt64
		errorInfo    sql.NullString
		createdAt    string
	)

	if err := db.QueryRowContext(ctx, q, requestID).Scan(
		&requestIDVal,
		&providerVal,
		&modelVal,
		&streamingVal,
		&tpsVal,
		&ttftVal,
		&tpotVal,
		&inTok,
		&outTok,
		&totalTok,
		&durMS,
		&statusCode,
		&errorInfo,
		&createdAt,
	); err != nil {
		return metricsRow{}, err
	}

	out := metricsRow{
		RequestID: requestIDVal,
		Provider:  providerVal,
		Model:     modelVal,
		Streaming: streamingVal != 0,
		CreatedAt: createdAt,
	}
	out.TPS = nullFloat64Ptr(tpsVal)
	out.DurationMS = nullInt64Ptr(durMS)
	out.StatusCode = nullInt64Ptr(statusCode)
	out.ErrorInfo = nullStringPtr(errorInfo)
	out.InputTokens = nullInt64Ptr(inTok)
	out.OutputTokens = nullInt64Ptr(outTok)
	out.TotalTokens = nullInt64Ptr(totalTok)
	if ttftVal.Valid {
		ms := secondsToMillisInt(ttftVal.Float64)
		out.TTFTMillis = &ms
	}
	if tpotVal.Valid {
		ms := secondsToMillisInt(tpotVal.Float64)
		out.TPOTMillis = &ms
	}
	return out, nil
}

func nullInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	out := v.Int64
	return &out
}

func nullFloat64Ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	out := v.Float64
	return &out
}

func nullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	out := v.String
	return &out
}
