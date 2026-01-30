package management

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metrics"
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

type metricsPercentileFloat struct {
	SampleCount int      `json:"sample_count"`
	P50         *float64 `json:"p50"`
	P95         *float64 `json:"p95"`
	P99         *float64 `json:"p99"`
}

type metricsPercentileMillis struct {
	SampleCount int    `json:"sample_count"`
	P50         *int64 `json:"p50"`
	P95         *int64 `json:"p95"`
	P99         *int64 `json:"p99"`
}

type metricsPercentilesMetrics struct {
	TPS        metricsPercentileFloat  `json:"tps"`
	TTFTMillis metricsPercentileMillis `json:"ttft_ms"`
	TPOTMillis metricsPercentileMillis `json:"tpot_ms"`
	DurationMS metricsPercentileMillis `json:"duration_ms"`
}

type metricsPercentilesGroup struct {
	Provider  string                    `json:"provider"`
	Model     string                    `json:"model"`
	Streaming bool                      `json:"streaming"`
	Count     int                       `json:"count"`
	Metrics   metricsPercentilesMetrics `json:"metrics"`
}

type metricsPercentilesResponse struct {
	Meta    metricsMeta               `json:"meta"`
	Success []metricsPercentilesGroup `json:"success"`
	Failure []metricsPercentilesGroup `json:"failure"`
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

	switch mode {
	case "percentiles":
		out, err := h.queryMetricsPercentiles(c.Request.Context(), effectiveFrom, effectiveTo, filters)
		if err != nil {
			c.JSON(http.StatusInternalServerError, metricsEnvelope[any]{
				Error: "failed to query metrics",
				Meta:  meta,
			})
			return
		}
		out.Meta = meta
		c.JSON(http.StatusOK, out)
		return
	default:
		c.JSON(http.StatusNotImplemented, metricsEnvelope[any]{
			Error: "mode not implemented",
			Meta:  meta,
		})
		return
	}
}

type metricsPercentilesAgg struct {
	Count      int
	TPS        []float64
	TTFTSec    []float64
	TPOTSec    []float64
	DurationMS []float64
}

type metricsPercentilesKey struct {
	Provider  string
	Model     string
	Streaming bool
}

func (h *Handler) queryMetricsPercentiles(ctx context.Context, from, to time.Time, filters metricsFilters) (metricsPercentilesResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db, err := h.openMetricsReadDB()
	if err != nil {
		// If the DB cannot be opened (e.g., default logs/ path missing in tests),
		// treat it as "no data" for percentiles mode.
		msg := err.Error()
		if strings.Contains(msg, "unable to open database file") || strings.Contains(msg, "no such file or directory") {
			return metricsPercentilesResponse{}, nil
		}
		return metricsPercentilesResponse{}, err
	}

	q := `SELECT
		provider,
		model,
		streaming,
		status_code,
		error_info,
		tps,
		ttft,
		tpot,
		duration_ms
	FROM metrics
	WHERE created_at >= datetime(?) AND created_at < datetime(?)`
	args := []any{from.Format(time.RFC3339), to.Format(time.RFC3339)}

	if filters.Provider != nil {
		q += " AND provider = ?"
		args = append(args, *filters.Provider)
	}
	if filters.Model != nil {
		q += " AND model = ?"
		args = append(args, *filters.Model)
	}
	if filters.Streaming != nil {
		q += " AND streaming = ?"
		if *filters.Streaming {
			args = append(args, int64(1))
		} else {
			args = append(args, int64(0))
		}
	}

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		// When schema is missing (e.g., a fresh DB file without migrations),
		// treat it as empty result rather than a hard 500.
		if strings.Contains(err.Error(), "no such table: metrics") {
			return metricsPercentilesResponse{}, nil
		}
		return metricsPercentilesResponse{}, err
	}
	defer func() { _ = rows.Close() }()

	successAgg := make(map[metricsPercentilesKey]*metricsPercentilesAgg)
	failureAgg := make(map[metricsPercentilesKey]*metricsPercentilesAgg)

	for rows.Next() {
		var (
			providerVal  string
			modelVal     string
			streamingVal int64
			statusCode   sql.NullInt64
			errorInfo    sql.NullString
			tpsVal       sql.NullFloat64
			ttftVal      sql.NullFloat64
			tpotVal      sql.NullFloat64
			durMS        sql.NullInt64
		)
		if err := rows.Scan(
			&providerVal,
			&modelVal,
			&streamingVal,
			&statusCode,
			&errorInfo,
			&tpsVal,
			&ttftVal,
			&tpotVal,
			&durMS,
		); err != nil {
			return metricsPercentilesResponse{}, err
		}

		key := metricsPercentilesKey{Provider: providerVal, Model: modelVal, Streaming: streamingVal != 0}
		isSuccess := statusCode.Valid && statusCode.Int64 >= 200 && statusCode.Int64 < 300
		if isSuccess {
			if errorInfo.Valid && strings.TrimSpace(errorInfo.String) != "" {
				isSuccess = false
			}
		}

		target := failureAgg
		if isSuccess {
			target = successAgg
		}
		agg := target[key]
		if agg == nil {
			agg = &metricsPercentilesAgg{}
			target[key] = agg
		}
		agg.Count++
		if tpsVal.Valid {
			agg.TPS = append(agg.TPS, tpsVal.Float64)
		}
		if ttftVal.Valid {
			agg.TTFTSec = append(agg.TTFTSec, ttftVal.Float64)
		}
		if tpotVal.Valid {
			agg.TPOTSec = append(agg.TPOTSec, tpotVal.Float64)
		}
		if durMS.Valid {
			agg.DurationMS = append(agg.DurationMS, float64(durMS.Int64))
		}
	}
	if err := rows.Err(); err != nil {
		return metricsPercentilesResponse{}, err
	}

	buildGroups := func(m map[metricsPercentilesKey]*metricsPercentilesAgg) []metricsPercentilesGroup {
		out := make([]metricsPercentilesGroup, 0, len(m))
		for key, agg := range m {
			g := metricsPercentilesGroup{
				Provider:  key.Provider,
				Model:     key.Model,
				Streaming: key.Streaming,
				Count:     agg.Count,
				Metrics:   buildMetricsPercentiles(agg),
			}
			out = append(out, g)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Provider != out[j].Provider {
				return out[i].Provider < out[j].Provider
			}
			if out[i].Model != out[j].Model {
				return out[i].Model < out[j].Model
			}
			if out[i].Streaming != out[j].Streaming {
				return !out[i].Streaming && out[j].Streaming
			}
			return false
		})
		return out
	}

	return metricsPercentilesResponse{
		Success: buildGroups(successAgg),
		Failure: buildGroups(failureAgg),
	}, nil
}

func buildMetricsPercentiles(agg *metricsPercentilesAgg) metricsPercentilesMetrics {
	var out metricsPercentilesMetrics
	if agg == nil {
		return out
	}

	out.TPS.SampleCount = len(agg.TPS)
	if p50, p95, p99, ok := metrics.CalculateP50P95P99(agg.TPS); ok {
		out.TPS.P50 = &p50
		out.TPS.P95 = &p95
		out.TPS.P99 = &p99
	}

	out.TTFTMillis.SampleCount = len(agg.TTFTSec)
	if p50, p95, p99, ok := metrics.CalculateP50P95P99(agg.TTFTSec); ok {
		p50ms := secondsToMillisInt(p50)
		p95ms := secondsToMillisInt(p95)
		p99ms := secondsToMillisInt(p99)
		out.TTFTMillis.P50 = &p50ms
		out.TTFTMillis.P95 = &p95ms
		out.TTFTMillis.P99 = &p99ms
	}

	out.TPOTMillis.SampleCount = len(agg.TPOTSec)
	if p50, p95, p99, ok := metrics.CalculateP50P95P99(agg.TPOTSec); ok {
		p50ms := secondsToMillisInt(p50)
		p95ms := secondsToMillisInt(p95)
		p99ms := secondsToMillisInt(p99)
		out.TPOTMillis.P50 = &p50ms
		out.TPOTMillis.P95 = &p95ms
		out.TPOTMillis.P99 = &p99ms
	}

	out.DurationMS.SampleCount = len(agg.DurationMS)
	if p50, p95, p99, ok := metrics.CalculateP50P95P99(agg.DurationMS); ok {
		p50ms := int64(math.Round(p50))
		p95ms := int64(math.Round(p95))
		p99ms := int64(math.Round(p99))
		out.DurationMS.P50 = &p50ms
		out.DurationMS.P95 = &p95ms
		out.DurationMS.P99 = &p99ms
	}

	return out
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
