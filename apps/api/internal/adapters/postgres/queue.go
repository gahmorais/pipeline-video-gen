package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// NotifyChannel é o canal LISTEN/NOTIFY de cada tipo de tarefa.
func NotifyChannel(kind ports.TaskKind) string { return "task_queue_" + string(kind) }

// taskProducer enfileira dentro da transação corrente. O NOTIFY só é entregue
// no commit, então workers nunca acordam para uma tarefa que foi desfeita.
type taskProducer struct {
	q querier
}

var _ ports.TaskQueue = (*taskProducer)(nil)

func (p *taskProducer) Enqueue(ctx context.Context, t ports.Task) error {
	// ON CONFLICT: já existe tarefa ativa igual → idempotente.
	_, err := p.q.ExecContext(ctx, `
		INSERT INTO task_queue (kind, job_id, render_id) VALUES ($1, $2, $3)
		ON CONFLICT (kind, job_id, render_id) WHERE status IN ('pending', 'running') DO NOTHING`,
		string(t.Kind), string(t.JobID), string(t.RenderID))
	if err != nil {
		return fmt.Errorf("inserindo tarefa: %w", err)
	}
	if _, err := p.q.ExecContext(ctx, `SELECT pg_notify($1, $2)`, NotifyChannel(t.Kind), string(t.JobID)); err != nil {
		return fmt.Errorf("notificando fila: %w", err)
	}
	return nil
}

// Queue é o consumidor da fila (SELECT ... FOR UPDATE SKIP LOCKED).
type Queue struct {
	db *sql.DB
}

var _ ports.TaskConsumer = (*Queue)(nil)

func NewQueue(db *sql.DB) *Queue { return &Queue{db: db} }

func (q *Queue) Claim(ctx context.Context, kind ports.TaskKind, workerID string) (*ports.ClaimedTask, error) {
	var (
		ct       ports.ClaimedTask
		jobID    string
		renderID string
	)
	err := q.db.QueryRowContext(ctx, `
		UPDATE task_queue SET status = 'running', locked_at = now(), locked_by = $2,
			attempts = attempts + 1, updated_at = now(),
			first_claimed_at = COALESCE(first_claimed_at, now()), last_claimed_at = now()
		WHERE id = (
			SELECT id FROM task_queue
			WHERE kind = $1 AND status = 'pending' AND run_after <= now()
			ORDER BY run_after, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1)
		RETURNING id, job_id, render_id, attempts`,
		string(kind), workerID,
	).Scan(&ct.ID, &jobID, &renderID, &ct.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reservando tarefa %s: %w", kind, err)
	}
	ct.Task = ports.Task{Kind: kind, JobID: domain.JobID(jobID), RenderID: domain.RenderID(renderID)}
	return &ct, nil
}

func (q *Queue) Complete(ctx context.Context, taskID int64) error {
	return q.expectOne(q.db.ExecContext(ctx, `
		UPDATE task_queue SET status = 'done', locked_at = NULL, finished_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'running'`, taskID))
}

func (q *Queue) Fail(ctx context.Context, taskID int64, cause error, retryDelay time.Duration) (bool, error) {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	var status string
	err := q.db.QueryRowContext(ctx, `
		UPDATE task_queue SET
			status = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'pending' END,
			run_after = now() + make_interval(secs => $3),
			locked_at = NULL, locked_by = NULL, last_error = $2, updated_at = now()
		WHERE id = $1 AND status = 'running'
		RETURNING status`, taskID, msg, retryDelay.Seconds()).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("tarefa %d não está em execução", taskID)
	}
	if err != nil {
		return false, err
	}
	return status == "dead", nil
}

func (q *Queue) Heartbeat(ctx context.Context, taskID int64) error {
	return q.expectOne(q.db.ExecContext(ctx, `
		UPDATE task_queue SET locked_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'running'`, taskID))
}

func (q *Queue) ReclaimStale(ctx context.Context, olderThan time.Duration) (int, error) {
	res, err := q.db.ExecContext(ctx, `
		UPDATE task_queue SET
			status = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'pending' END,
			locked_at = NULL, locked_by = NULL,
			last_error = 'worker parou de responder (reclaim)', updated_at = now()
		WHERE status = 'running' AND locked_at < now() - make_interval(secs => $1)`,
		olderThan.Seconds())
	if err != nil {
		return 0, fmt.Errorf("reclaim: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}

func (q *Queue) expectOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("tarefa não encontrada ou não está em execução")
	}
	return nil
}
