package ports

import (
	"context"
	"time"
)

// MetricsReader fornece o resumo operacional (read model, consultado sem passar pelo agregado).
type MetricsReader interface {
	Summary(ctx context.Context, since time.Time) (MetricsSummary, error)
}

// MetricsSummary é o resumo operacional. Durações em segundos.
type MetricsSummary struct {
	Since  time.Time         `json:"since"`
	Jobs   map[string]int    `json:"jobsByStatus"`
	Queues []QueueMetrics    `json:"queues"`
	Render RenderMetrics     `json:"render"`
	Fails  []TemplateFailure `json:"failureRateByTemplate"`
	LLM    []LLMModelMetrics `json:"llm"`
}

type QueueMetrics struct {
	Kind              string  `json:"kind"`
	Done              int     `json:"done"`
	Dead              int     `json:"dead"`
	Pending           int     `json:"pending"`
	Running           int     `json:"running"`
	AvgWaitSeconds    float64 `json:"avgWaitSeconds"`
	P95WaitSeconds    float64 `json:"p95WaitSeconds"`
	AvgProcessSeconds float64 `json:"avgProcessSeconds"`
}

type RenderMetrics struct {
	Renders int `json:"renders"`
	// Segundos de render por segundo de vídeo: base do ETA na UI.
	SecondsPerVideoSecond float64 `json:"renderSecondsPerVideoSecond"`
}

type TemplateFailure struct {
	Template    string  `json:"template"`
	Total       int     `json:"total"`
	Failed      int     `json:"failed"`
	FailureRate float64 `json:"failureRate"`
}

type LLMModelMetrics struct {
	Model        string  `json:"model"`
	Calls        int     `json:"calls"`
	Failures     int     `json:"failures"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CostUSD      float64 `json:"costUsd"`
	AvgSeconds   float64 `json:"avgSeconds"`
}
