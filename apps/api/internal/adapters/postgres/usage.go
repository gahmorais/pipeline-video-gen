package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// UsageRecorder grava métricas de uso do LLM na tabela llm_usage.
type UsageRecorder struct {
	db *sql.DB
}

var _ ports.LLMUsageRecorder = (*UsageRecorder)(nil)

func NewUsageRecorder(db *sql.DB) *UsageRecorder { return &UsageRecorder{db: db} }

func (r *UsageRecorder) Record(ctx context.Context, u ports.LLMUsage) error {
	var errText sql.NullString
	if u.Error != "" {
		errText = sql.NullString{String: u.Error, Valid: true}
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO llm_usage (job_id, model, input_tokens, output_tokens, cost_usd, duration_ms, success, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		string(u.JobID), u.Model, u.InputTokens, u.OutputTokens, u.CostUSD, u.Duration.Milliseconds(), u.Success, errText)
	if err != nil {
		return fmt.Errorf("gravando uso do llm: %w", err)
	}
	return nil
}

// JobCostUSD soma o custo estimado de todas as chamadas de um job.
func (r *UsageRecorder) JobCostUSD(ctx context.Context, jobID string) (float64, error) {
	var total float64
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost_usd), 0)::float8 FROM llm_usage WHERE job_id = $1`, jobID).Scan(&total)
	return total, err
}
