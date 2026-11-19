package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/worker"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

type fakeQueue struct {
	mu        sync.Mutex
	pending   []ports.ClaimedTask
	completed []int64
	failed    []int64
	delays    []time.Duration
	dead      bool
	claimErr  error
	reclaimed int
}

func (q *fakeQueue) Claim(context.Context, ports.TaskKind, string) (*ports.ClaimedTask, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.claimErr != nil {
		return nil, q.claimErr
	}
	if len(q.pending) == 0 {
		return nil, nil
	}
	t := q.pending[0]
	q.pending = q.pending[1:]
	return &t, nil
}

func (q *fakeQueue) Complete(_ context.Context, id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.completed = append(q.completed, id)
	return nil
}

func (q *fakeQueue) Fail(_ context.Context, id int64, _ error, d time.Duration) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.failed = append(q.failed, id)
	q.delays = append(q.delays, d)
	return q.dead, nil
}

func (q *fakeQueue) Heartbeat(context.Context, int64) error { return nil }

func (q *fakeQueue) ReclaimStale(context.Context, time.Duration) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reclaimed++
	return 0, nil
}

type fakeProc struct {
	mu        sync.Mutex
	err       error
	generated []domain.JobID
	failed    []domain.JobID
}

func (p *fakeProc) GenerateStoryboard(_ context.Context, id domain.JobID) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.generated = append(p.generated, id)
	return p.err
}

func (p *fakeProc) FailStoryboard(_ context.Context, id domain.JobID, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failed = append(p.failed, id)
	return nil
}

type noWait struct{}

func (noWait) Wait(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
	case <-time.After(time.Millisecond):
	}
	return nil
}

func task(id int64, job string, attempts int) ports.ClaimedTask {
	return ports.ClaimedTask{ID: id, Attempts: attempts, Task: ports.Task{Kind: ports.TaskGenerateStoryboard, JobID: domain.JobID(job)}}
}

func newWorker(q *fakeQueue, p *fakeProc) *worker.StoryboardWorker {
	return worker.NewStoryboardWorker(q, p, noWait{}, worker.StoryboardConfig{RetryBase: time.Second}, discard)
}

func TestRunOnceCompletesTask(t *testing.T) {
	q, p := &fakeQueue{pending: []ports.ClaimedTask{task(1, "j1", 1)}}, &fakeProc{}
	processed, err := newWorker(q, p).RunOnce(context.Background(), "w")
	if err != nil || !processed || len(q.completed) != 1 || p.generated[0] != "j1" {
		t.Fatalf("processed=%v err=%v q=%+v", processed, err, q)
	}
}

func TestRunOnceEmptyQueue(t *testing.T) {
	processed, err := newWorker(&fakeQueue{}, &fakeProc{}).RunOnce(context.Background(), "w")
	if processed || err != nil {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
}

func TestRunOnceInfraErrorRetriesWithBackoff(t *testing.T) {
	q := &fakeQueue{pending: []ports.ClaimedTask{task(1, "j1", 1), task(2, "j2", 3)}}
	p := &fakeProc{err: errors.New("db down")}
	w := newWorker(q, p)
	_, _ = w.RunOnce(context.Background(), "w")
	_, _ = w.RunOnce(context.Background(), "w")
	if len(q.failed) != 2 || q.delays[0] != time.Second || q.delays[1] != 4*time.Second {
		t.Fatalf("backoff: %v", q.delays)
	}
	if len(p.failed) != 0 {
		t.Fatal("job não deve falhar enquanto houver tentativas")
	}
}

func TestRunOnceDeadTaskFailsJob(t *testing.T) {
	q := &fakeQueue{pending: []ports.ClaimedTask{task(1, "j1", 3)}, dead: true}
	p := &fakeProc{err: errors.New("db down")}
	if _, err := newWorker(q, p).RunOnce(context.Background(), "w"); err != nil {
		t.Fatal(err)
	}
	if len(p.failed) != 1 || p.failed[0] != "j1" {
		t.Fatalf("job deve ir para FAILED quando a tarefa morre: %v", p.failed)
	}
}

func TestRunProcessesAllAndStopsOnCancel(t *testing.T) {
	q := &fakeQueue{}
	for i := int64(1); i <= 20; i++ {
		q.pending = append(q.pending, task(i, "j", 1))
	}
	p := &fakeProc{}
	w := worker.NewStoryboardWorker(q, p, noWait{}, worker.StoryboardConfig{Concurrency: 4}, discard)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		q.mu.Lock()
		n := len(q.completed)
		q.mu.Unlock()
		if n == 20 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("processadas %d de 20", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run não encerrou após cancelamento")
	}
}

func TestMaintenanceReclaimsPeriodically(t *testing.T) {
	q := &fakeQueue{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	worker.Maintenance{Queue: q, Interval: 10 * time.Millisecond, StaleAfter: time.Minute, Logger: discard}.Run(ctx)
	if q.reclaimed < 3 {
		t.Fatalf("reclaim executado %d vezes", q.reclaimed)
	}
}
