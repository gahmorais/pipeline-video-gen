package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// querier é satisfeito por *sql.DB e *sql.Tx: os adapters funcionam dentro ou fora de transação.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// jobRepository persiste o agregado via Snapshot/Rehydrate (o domínio não conhece SQL).
type jobRepository struct {
	q querier
}

var _ ports.VideoJobRepository = (*jobRepository)(nil)

const selectJob = `
SELECT id, prompt, template_id, template_version, status, storyboard, render_id,
       render_progress, output_key, failure_reason, failure_from, failed_at,
       retry_count, version, created_at, updated_at
FROM video_jobs WHERE id = $1`

func (r *jobRepository) Get(ctx context.Context, id domain.JobID) (*domain.VideoJob, error) {
	var (
		s                    domain.Snapshot
		storyboard           []byte
		failReason, failFrom sql.NullString
		failedAt             sql.NullTime
		templateID           string
		templateVersion      int
	)
	err := r.q.QueryRowContext(ctx, selectJob, string(id)).Scan(
		&s.ID, &s.Prompt, &templateID, &templateVersion, &s.Status, &storyboard, &s.RenderID,
		&s.RenderProgress, &s.OutputKey, &failReason, &failFrom, &failedAt,
		&s.RetryCount, &s.Version, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("buscando job %s: %w", id, err)
	}
	s.Template = domain.TemplateRef{ID: templateID, Version: templateVersion}
	s.Storyboard = storyboard
	if failReason.Valid {
		s.Failure = &domain.Failure{Reason: failReason.String, From: domain.JobStatus(failFrom.String), At: failedAt.Time}
	}
	return domain.Rehydrate(s)
}

func (r *jobRepository) Create(ctx context.Context, job *domain.VideoJob) error {
	s := job.Snapshot()
	reason, from, at := failureColumns(s.Failure)
	_, err := r.q.ExecContext(ctx, `
		INSERT INTO video_jobs (id, prompt, template_id, template_version, status, storyboard,
			render_id, render_progress, output_key, failure_reason, failure_from, failed_at,
			retry_count, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11, $12, $13, 1, $14, $15)`,
		string(s.ID), string(s.Prompt), s.Template.ID, s.Template.Version, string(s.Status), nullableJSON(s.Storyboard),
		string(s.RenderID), s.RenderProgress, s.OutputKey, reason, from, at,
		s.RetryCount, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserindo job %s: %w", s.ID, err)
	}
	job.SetVersion(1)
	return nil
}

// Update grava com controle otimista: só altera se a versão no banco for a lida.
func (r *jobRepository) Update(ctx context.Context, job *domain.VideoJob) error {
	s := job.Snapshot()
	reason, from, at := failureColumns(s.Failure)
	res, err := r.q.ExecContext(ctx, `
		UPDATE video_jobs SET
			status = $3, storyboard = $4::jsonb, render_id = $5, render_progress = $6,
			output_key = $7, failure_reason = $8, failure_from = $9, failed_at = $10,
			retry_count = $11, updated_at = $12, version = version + 1
		WHERE id = $1 AND version = $2`,
		string(s.ID), s.Version, string(s.Status), nullableJSON(s.Storyboard), string(s.RenderID), s.RenderProgress,
		s.OutputKey, reason, from, at, s.RetryCount, s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("atualizando job %s: %w", s.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists bool
		if err := r.q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM video_jobs WHERE id = $1)`, string(s.ID)).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: %s", domain.ErrNotFound, s.ID)
		}
		return fmt.Errorf("%w: %s (versão %d)", domain.ErrConcurrentUpdate, s.ID, s.Version)
	}
	job.SetVersion(s.Version + 1)
	return nil
}

func failureColumns(f *domain.Failure) (reason, from sql.NullString, at sql.NullTime) {
	if f == nil {
		return
	}
	return sql.NullString{String: f.Reason, Valid: true},
		sql.NullString{String: string(f.From), Valid: true},
		sql.NullTime{Time: f.At, Valid: true}
}

func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}
