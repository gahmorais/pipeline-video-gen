// Package worker contém os loops que consomem a fila e acionam os use cases
// (driving adapters, como o HTTP). Roda como goroutines dentro do processo da API.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// StoryboardProcessor é o subconjunto de use cases usado pelo worker.
type StoryboardProcessor interface {
	GenerateStoryboard(ctx context.Context, id domain.JobID) error
	FailStoryboard(ctx context.Context, id domain.JobID, reason string) error
}

// Waiter bloqueia até haver possível trabalho novo (LISTEN/NOTIFY) ou o intervalo de polling.
type Waiter interface {
	Wait(ctx context.Context, pollInterval time.Duration) error
}

type StoryboardConfig struct {
	WorkerID     string
	Concurrency  int           // chamadas ao LLM em paralelo (I/O-bound)
	PollInterval time.Duration // polling de segurança caso uma notificação se perca
	TaskTimeout  time.Duration // tempo máximo de uma geração
	RetryBase    time.Duration // backoff base para falhas de infraestrutura
}

func (c *StoryboardConfig) defaults() {
	if c.Concurrency <= 0 {
		c.Concurrency = 2
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 5 * time.Second
	}
	if c.TaskTimeout <= 0 {
		c.TaskTimeout = 3 * time.Minute
	}
	if c.RetryBase <= 0 {
		c.RetryBase = 5 * time.Second
	}
	if c.WorkerID == "" {
		c.WorkerID = "storyboard-worker"
	}
}

type StoryboardWorker struct {
	queue  ports.TaskConsumer
	proc   StoryboardProcessor
	waiter Waiter
	cfg    StoryboardConfig
	logger *slog.Logger
}

func NewStoryboardWorker(queue ports.TaskConsumer, proc StoryboardProcessor, waiter Waiter, cfg StoryboardConfig, logger *slog.Logger) *StoryboardWorker {
	cfg.defaults()
	return &StoryboardWorker{queue: queue, proc: proc, waiter: waiter, cfg: cfg, logger: logger}
}

// Run bloqueia até o contexto ser cancelado; tarefas em andamento terminam antes de retornar.
func (w *StoryboardWorker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < w.cfg.Concurrency; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			w.loop(ctx, fmt.Sprintf("%s-%d", w.cfg.WorkerID, slot))
		}(i)
	}
	wg.Wait()
}

func (w *StoryboardWorker) loop(ctx context.Context, workerID string) {
	for ctx.Err() == nil {
		processed, err := w.RunOnce(ctx, workerID)
		if err != nil {
			w.logger.ErrorContext(ctx, "storyboard worker", "worker_id", workerID, "err", err)
			sleep(ctx, w.cfg.RetryBase)
			continue
		}
		if !processed {
			_ = w.waiter.Wait(ctx, w.cfg.PollInterval)
		}
	}
}

// RunOnce reserva e processa no máximo uma tarefa. Exportado para testes e uso síncrono.
func (w *StoryboardWorker) RunOnce(ctx context.Context, workerID string) (processed bool, err error) {
	task, err := w.queue.Claim(ctx, ports.TaskGenerateStoryboard, workerID)
	if err != nil || task == nil {
		return false, err
	}
	log := w.logger.With("job_id", task.Task.JobID, "task_id", task.ID, "attempt", task.Attempts)

	// A tarefa termina mesmo se o processo estiver encerrando (graceful shutdown).
	taskCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.cfg.TaskTimeout)
	defer cancel()

	start := time.Now()
	procErr := w.proc.GenerateStoryboard(taskCtx, task.Task.JobID)
	// Persistir o resultado da fila não deve depender do ctx do loop.
	ackCtx, ackCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer ackCancel()

	if procErr == nil {
		log.InfoContext(ctx, "roteiro processado", "duration_ms", time.Since(start).Milliseconds())
		return true, w.queue.Complete(ackCtx, task.ID)
	}

	// Erros do LLM já viram FAILED no use case; aqui chegam erros de infraestrutura.
	delay := w.cfg.RetryBase * time.Duration(1<<min(task.Attempts-1, 6))
	dead, failErr := w.queue.Fail(ackCtx, task.ID, procErr, delay)
	if failErr != nil {
		return true, errors.Join(procErr, failErr)
	}
	log.WarnContext(ctx, "falha ao processar roteiro", "err", procErr, "dead", dead, "retry_in", delay)
	if dead {
		if err := w.proc.FailStoryboard(ackCtx, task.Task.JobID, "tentativas esgotadas: "+procErr.Error()); err != nil {
			return true, err
		}
	}
	return true, nil
}

// Maintenance devolve à fila tarefas de workers que pararam de responder.
// Roda em um único lugar (a API) para todos os tipos de tarefa.
type Maintenance struct {
	Queue      ports.TaskConsumer
	Interval   time.Duration
	StaleAfter time.Duration
	Logger     *slog.Logger
}

func (m Maintenance) Run(ctx context.Context) {
	ticker := time.NewTicker(m.Interval)
	defer ticker.Stop()
	for {
		n, err := m.Queue.ReclaimStale(ctx, m.StaleAfter)
		switch {
		case err != nil && ctx.Err() == nil:
			m.Logger.ErrorContext(ctx, "reclaim de tarefas", "err", err)
		case n > 0:
			m.Logger.WarnContext(ctx, "tarefas abandonadas devolvidas à fila", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
