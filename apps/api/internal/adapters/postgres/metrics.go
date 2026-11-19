package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// MetricsReader implementa ports.MetricsReader com consultas SQL.
type MetricsReader struct {
	db *sql.DB
}

var _ ports.MetricsReader = (*MetricsReader)(nil)

func NewMetricsReader(db *sql.DB) *MetricsReader { return &MetricsReader{db: db} }

func (m *MetricsReader) Summary(ctx context.Context, since time.Time) (ports.MetricsSummary, error) {
	// Listas sempre não-nulas: o JSON devolve [] em vez de null.
	out := ports.MetricsSummary{
		Since:  since,
		Jobs:   map[string]int{},
		Queues: []ports.QueueMetrics{},
		Fails:  []ports.TemplateFailure{},
		LLM:    []ports.LLMModelMetrics{},
	}
	steps := []func(context.Context, *ports.MetricsSummary) error{m.jobs, m.queues, m.render, m.failures, m.llm}
	for _, step := range steps {
		if err := step(ctx, &out); err != nil {
			return out, fmt.Errorf("métricas: %w", err)
		}
	}
	return out, nil
}

func (m *MetricsReader) jobs(ctx context.Context, out *ports.MetricsSummary) error {
	rows, err := m.db.QueryContext(ctx,
		`SELECT status, count(*) FROM video_jobs WHERE created_at >= $1 GROUP BY status`, out.Since)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return err
		}
		out.Jobs[status] = n
	}
	return rows.Err()
}

func (m *MetricsReader) queues(ctx context.Context, out *ports.MetricsSummary) error {
	rows, err := m.db.QueryContext(ctx, `
		SELECT kind,
		       count(*) FILTER (WHERE status = 'done'),
		       count(*) FILTER (WHERE status = 'dead'),
		       count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'running'),
		       COALESCE(avg(EXTRACT(EPOCH FROM first_claimed_at - created_at)), 0)::float8,
		       COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM first_claimed_at - created_at)), 0)::float8,
		       COALESCE(avg(EXTRACT(EPOCH FROM finished_at - last_claimed_at)) FILTER (WHERE status = 'done'), 0)::float8
		FROM task_queue WHERE created_at >= $1
		GROUP BY kind ORDER BY kind`, out.Since)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var q ports.QueueMetrics
		if err := rows.Scan(&q.Kind, &q.Done, &q.Dead, &q.Pending, &q.Running, &q.AvgWaitSeconds, &q.P95WaitSeconds, &q.AvgProcessSeconds); err != nil {
			return err
		}
		out.Queues = append(out.Queues, q)
	}
	return rows.Err()
}

// render: razão entre tempo de processamento e duração do vídeo (soma de durationSec das cenas).
func (m *MetricsReader) render(ctx context.Context, out *ports.MetricsSummary) error {
	return m.db.QueryRowContext(ctx, `
		WITH r AS (
			SELECT EXTRACT(EPOCH FROM t.finished_at - t.last_claimed_at) AS render_s,
			       (SELECT sum((s->>'durationSec')::float8) FROM jsonb_array_elements(v.storyboard->'scenes') s) AS video_s
			FROM task_queue t JOIN video_jobs v ON v.id = t.job_id
			WHERE t.kind = 'render' AND t.status = 'done' AND t.finished_at >= $1 AND v.storyboard IS NOT NULL
		)
		SELECT count(*), COALESCE(sum(render_s) / NULLIF(sum(video_s), 0), 0)::float8 FROM r WHERE video_s > 0`,
		out.Since).Scan(&out.Render.Renders, &out.Render.SecondsPerVideoSecond)
}

func (m *MetricsReader) failures(ctx context.Context, out *ports.MetricsSummary) error {
	rows, err := m.db.QueryContext(ctx, `
		SELECT template_id || '@' || template_version, count(*), count(*) FILTER (WHERE status = 'FAILED')
		FROM video_jobs WHERE created_at >= $1
		GROUP BY template_id, template_version ORDER BY 1`, out.Since)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var f ports.TemplateFailure
		if err := rows.Scan(&f.Template, &f.Total, &f.Failed); err != nil {
			return err
		}
		if f.Total > 0 {
			f.FailureRate = float64(f.Failed) / float64(f.Total)
		}
		out.Fails = append(out.Fails, f)
	}
	return rows.Err()
}

func (m *MetricsReader) llm(ctx context.Context, out *ports.MetricsSummary) error {
	rows, err := m.db.QueryContext(ctx, `
		SELECT model, count(*), count(*) FILTER (WHERE NOT success),
		       COALESCE(sum(input_tokens), 0), COALESCE(sum(output_tokens), 0),
		       COALESCE(sum(cost_usd), 0)::float8, COALESCE(avg(duration_ms) / 1000.0, 0)::float8
		FROM llm_usage WHERE created_at >= $1
		GROUP BY model ORDER BY model`, out.Since)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var l ports.LLMModelMetrics
		if err := rows.Scan(&l.Model, &l.Calls, &l.Failures, &l.InputTokens, &l.OutputTokens, &l.CostUSD, &l.AvgSeconds); err != nil {
			return err
		}
		out.LLM = append(out.LLM, l)
	}
	return rows.Err()
}
