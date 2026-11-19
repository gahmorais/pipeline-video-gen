package postgres_test

// Testes de integração contra um PostgreSQL real.
// Pulados quando DATABASE_URL não está definido. Exemplo local:
//
//	docker compose up -d postgres
//	DATABASE_URL=postgres://videogen:videogen@localhost:5432/videogen?sslmode=disable go test ./internal/adapters/postgres/...
//
// ATENÇÃO: os testes apagam as tabelas do banco apontado.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/postgres"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/application"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

var t0 = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func setupDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de integração")
	}
	ctx := context.Background()
	db, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS task_queue, video_jobs, schema_migrations CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db, dsn
}

func newJob(t *testing.T, id string) *domain.VideoJob {
	t.Helper()
	tpl, _ := domain.NewTemplateRef("promo", 1)
	j, err := domain.NewVideoJob(domain.JobID(id), "vídeo de ofertas", tpl, t0)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func createJob(t *testing.T, uow *postgres.UnitOfWork, j *domain.VideoJob, tasks ...ports.Task) {
	t.Helper()
	err := uow.Do(context.Background(), func(ctx context.Context, r ports.Repositories) error {
		if err := r.Jobs.Create(ctx, j); err != nil {
			return err
		}
		for _, task := range tasks {
			if err := r.Queue.Enqueue(ctx, task); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db, _ := setupDB(t)
	if err := postgres.Migrate(context.Background(), db); err != nil {
		t.Fatalf("segunda migração: %v", err)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&n)
	if n != 1 {
		t.Fatalf("esperado 1 migração registrada, obtido %d", n)
	}
}

func TestRepositoryRoundTripAllStates(t *testing.T) {
	db, _ := setupDB(t)
	uow := postgres.NewUnitOfWork(db)
	ctx := context.Background()

	j := newJob(t, "job-rt")
	createJob(t, uow, j)

	sb, _ := domain.NewStoryboard([]byte(`{"schemaVersion": 1, "scenes": [{"durationSec": 1.5}]}`))
	steps := []func(*domain.VideoJob) error{
		func(j *domain.VideoJob) error { return j.StartStoryboardGeneration(t0.Add(time.Second)) },
		func(j *domain.VideoJob) error { return j.AttachStoryboard(sb, t0.Add(2*time.Second)) },
		func(j *domain.VideoJob) error { _, err := j.RequestRender("r1", t0); return err },
		func(j *domain.VideoJob) error { return j.StartRendering("r1", t0) },
		func(j *domain.VideoJob) error { _, err := j.ReportRenderProgress("r1", 0.4, t0); return err },
		func(j *domain.VideoJob) error { return j.Fail("chrome crashed", t0.Add(3*time.Second)) },
	}
	for i, step := range steps {
		err := uow.Do(ctx, func(ctx context.Context, r ports.Repositories) error {
			job, err := r.Jobs.Get(ctx, "job-rt")
			if err != nil {
				return err
			}
			if err := step(job); err != nil {
				return err
			}
			return r.Jobs.Update(ctx, job)
		})
		if err != nil {
			t.Fatalf("passo %d: %v", i, err)
		}
	}

	var got *domain.VideoJob
	_ = uow.Do(ctx, func(ctx context.Context, r ports.Repositories) error {
		var err error
		got, err = r.Jobs.Get(ctx, "job-rt")
		return err
	})
	s := got.Snapshot()
	if s.Status != domain.StatusFailed || s.RenderID != "r1" || s.RenderProgress != 0.4 || s.Version != 7 {
		t.Fatalf("snapshot: %+v", s)
	}
	if s.Failure == nil || s.Failure.Reason != "chrome crashed" || s.Failure.From != domain.StatusRendering ||
		!s.Failure.At.Equal(t0.Add(3*time.Second)) {
		t.Fatalf("falha: %+v", s.Failure)
	}
	if s.Template.String() != "promo@1" || s.Prompt != "vídeo de ofertas" || !s.CreatedAt.Equal(t0) {
		t.Fatalf("campos imutáveis: %+v", s)
	}
	// JSONB normaliza espaços; o conteúdo semântico deve ser preservado.
	if string(s.Storyboard) != `{"scenes": [{"durationSec": 1.5}], "schemaVersion": 1}` {
		t.Fatalf("storyboard: %s", s.Storyboard)
	}
}

func TestGetNotFound(t *testing.T) {
	db, _ := setupDB(t)
	err := postgres.NewUnitOfWork(db).Do(context.Background(), func(ctx context.Context, r ports.Repositories) error {
		_, err := r.Jobs.Get(ctx, "nope")
		return err
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("esperado ErrNotFound, obtido %v", err)
	}
}

func TestOptimisticConcurrency(t *testing.T) {
	db, _ := setupDB(t)
	uow := postgres.NewUnitOfWork(db)
	ctx := context.Background()
	createJob(t, uow, newJob(t, "job-cc"))

	load := func() *domain.VideoJob {
		var j *domain.VideoJob
		_ = uow.Do(ctx, func(ctx context.Context, r ports.Repositories) error {
			var err error
			j, err = r.Jobs.Get(ctx, "job-cc")
			return err
		})
		return j
	}
	a, b := load(), load()
	_ = a.StartStoryboardGeneration(t0)
	_ = b.Fail("x", t0)

	save := func(j *domain.VideoJob) error {
		return uow.Do(ctx, func(ctx context.Context, r ports.Repositories) error { return r.Jobs.Update(ctx, j) })
	}
	if err := save(a); err != nil {
		t.Fatal(err)
	}
	if err := save(b); !errors.Is(err, domain.ErrConcurrentUpdate) {
		t.Fatalf("esperado ErrConcurrentUpdate, obtido %v", err)
	}
}

func TestUnitOfWorkRollsBackJobAndQueue(t *testing.T) {
	db, _ := setupDB(t)
	uow := postgres.NewUnitOfWork(db)
	boom := errors.New("boom")
	err := uow.Do(context.Background(), func(ctx context.Context, r ports.Repositories) error {
		if err := r.Jobs.Create(ctx, newJob(t, "job-rb")); err != nil {
			return err
		}
		if err := r.Queue.Enqueue(ctx, ports.Task{Kind: ports.TaskGenerateStoryboard, JobID: "job-rb"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("esperado boom, obtido %v", err)
	}
	var jobs, tasks int
	_ = db.QueryRow(`SELECT count(*) FROM video_jobs`).Scan(&jobs)
	_ = db.QueryRow(`SELECT count(*) FROM task_queue`).Scan(&tasks)
	if jobs != 0 || tasks != 0 {
		t.Fatalf("rollback incompleto: jobs=%d tasks=%d", jobs, tasks)
	}
}

func TestEnqueueIsIdempotentWhileActive(t *testing.T) {
	db, _ := setupDB(t)
	uow := postgres.NewUnitOfWork(db)
	task := ports.Task{Kind: ports.TaskRender, JobID: "job-dup", RenderID: "r1"}
	createJob(t, uow, newJob(t, "job-dup"), task, task)

	q := postgres.NewQueue(db)
	ctx := context.Background()
	claimed, _ := q.Claim(ctx, ports.TaskRender, "w1")
	if claimed == nil {
		t.Fatal("esperado uma tarefa")
	}
	if again, _ := q.Claim(ctx, ports.TaskRender, "w1"); again != nil {
		t.Fatal("tarefa duplicada foi enfileirada")
	}
	// Após concluída, o mesmo trabalho pode ser enfileirado de novo (ex.: retry).
	_ = q.Complete(ctx, claimed.ID)
	_ = uow.Do(ctx, func(ctx context.Context, r ports.Repositories) error { return r.Queue.Enqueue(ctx, task) })
	if next, _ := q.Claim(ctx, ports.TaskRender, "w1"); next == nil {
		t.Fatal("reenfileiramento após conclusão deve ser aceito")
	}
}

func TestClaimFiltersByKindAndOrders(t *testing.T) {
	db, _ := setupDB(t)
	uow := postgres.NewUnitOfWork(db)
	createJob(t, uow, newJob(t, "a"), ports.Task{Kind: ports.TaskGenerateStoryboard, JobID: "a"})
	createJob(t, uow, newJob(t, "b"), ports.Task{Kind: ports.TaskRender, JobID: "b", RenderID: "rb"})
	createJob(t, uow, newJob(t, "c"), ports.Task{Kind: ports.TaskGenerateStoryboard, JobID: "c"})

	q := postgres.NewQueue(db)
	ctx := context.Background()
	first, _ := q.Claim(ctx, ports.TaskGenerateStoryboard, "w")
	second, _ := q.Claim(ctx, ports.TaskGenerateStoryboard, "w")
	third, _ := q.Claim(ctx, ports.TaskGenerateStoryboard, "w")
	if first.Task.JobID != "a" || second.Task.JobID != "c" || third != nil {
		t.Fatalf("ordem/filtro incorretos: %+v %+v %+v", first, second, third)
	}
	render, _ := q.Claim(ctx, ports.TaskRender, "w")
	if render == nil || render.Task.RenderID != "rb" || render.Attempts != 1 {
		t.Fatalf("render: %+v", render)
	}
}

func TestConcurrentClaimsNeverDuplicate(t *testing.T) {
	db, _ := setupDB(t)
	uow := postgres.NewUnitOfWork(db)
	const total = 40
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("job-%02d", i)
		createJob(t, uow, newJob(t, id), ports.Task{Kind: ports.TaskGenerateStoryboard, JobID: domain.JobID(id)})
	}

	q := postgres.NewQueue(db)
	var (
		mu   sync.Mutex
		seen = map[int64]int{}
		wg   sync.WaitGroup
	)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for {
				ct, err := q.Claim(context.Background(), ports.TaskGenerateStoryboard, fmt.Sprintf("w%d", worker))
				if err != nil {
					t.Error(err)
					return
				}
				if ct == nil {
					return
				}
				mu.Lock()
				seen[ct.ID]++
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	if len(seen) != total {
		t.Fatalf("esperado %d tarefas reservadas, obtido %d", total, len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("tarefa %d reservada %d vezes", id, n)
		}
	}
}

func TestFailRetriesWithDelayThenDies(t *testing.T) {
	db, _ := setupDB(t)
	createJob(t, postgres.NewUnitOfWork(db), newJob(t, "job-f"), ports.Task{Kind: ports.TaskGenerateStoryboard, JobID: "job-f"})
	q := postgres.NewQueue(db)
	ctx := context.Background()

	for attempt := 1; attempt <= 3; attempt++ {
		ct, err := q.Claim(ctx, ports.TaskGenerateStoryboard, "w")
		if err != nil || ct == nil {
			t.Fatalf("tentativa %d: claim %v %v", attempt, ct, err)
		}
		if ct.Attempts != attempt {
			t.Fatalf("attempts=%d, esperado %d", ct.Attempts, attempt)
		}
		dead, err := q.Fail(ctx, ct.ID, errors.New("llm 503"), 0)
		if err != nil {
			t.Fatal(err)
		}
		if dead != (attempt == 3) {
			t.Fatalf("tentativa %d: dead=%v", attempt, dead)
		}
	}
	if ct, _ := q.Claim(ctx, ports.TaskGenerateStoryboard, "w"); ct != nil {
		t.Fatal("tarefa morta não pode ser reservada")
	}
	var lastErr string
	_ = db.QueryRow(`SELECT last_error FROM task_queue WHERE job_id = 'job-f'`).Scan(&lastErr)
	if lastErr != "llm 503" {
		t.Fatalf("last_error=%q", lastErr)
	}
}

func TestFailRespectsRetryDelay(t *testing.T) {
	db, _ := setupDB(t)
	createJob(t, postgres.NewUnitOfWork(db), newJob(t, "job-d"), ports.Task{Kind: ports.TaskGenerateStoryboard, JobID: "job-d"})
	q := postgres.NewQueue(db)
	ctx := context.Background()
	ct, _ := q.Claim(ctx, ports.TaskGenerateStoryboard, "w")
	_, _ = q.Fail(ctx, ct.ID, errors.New("x"), time.Hour)
	if again, _ := q.Claim(ctx, ports.TaskGenerateStoryboard, "w"); again != nil {
		t.Fatal("tarefa com backoff não pode ser reservada antes do run_after")
	}
}

func TestReclaimStaleAndHeartbeat(t *testing.T) {
	db, _ := setupDB(t)
	uow := postgres.NewUnitOfWork(db)
	createJob(t, uow, newJob(t, "stale"), ports.Task{Kind: ports.TaskRender, JobID: "stale", RenderID: "r"})
	createJob(t, uow, newJob(t, "alive"), ports.Task{Kind: ports.TaskRender, JobID: "alive", RenderID: "r"})
	q := postgres.NewQueue(db)
	ctx := context.Background()

	a, _ := q.Claim(ctx, ports.TaskRender, "morto")
	b, _ := q.Claim(ctx, ports.TaskRender, "vivo")
	// Simula o worker "morto" sem sinal há 10 minutos.
	_, _ = db.Exec(`UPDATE task_queue SET locked_at = now() - interval '10 minutes'`)
	if err := q.Heartbeat(ctx, b.ID); err != nil {
		t.Fatal(err)
	}

	n, err := q.ReclaimStale(ctx, 5*time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("reclaim: n=%d err=%v", n, err)
	}
	again, _ := q.Claim(ctx, ports.TaskRender, "novo")
	if again == nil || again.ID != a.ID || again.Attempts != 2 {
		t.Fatalf("tarefa abandonada deveria voltar à fila: %+v", again)
	}
	if err := q.Heartbeat(ctx, 999999); err == nil {
		t.Fatal("heartbeat de tarefa inexistente deve falhar")
	}
}

func TestListenerWakesOnCommitOnly(t *testing.T) {
	db, dsn := setupDB(t)
	uow := postgres.NewUnitOfWork(db)
	l, err := postgres.NewListener(dsn, ports.TaskRender, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	time.Sleep(100 * time.Millisecond) // LISTEN ativo

	// Transação desfeita: nenhuma notificação.
	_ = uow.Do(context.Background(), func(ctx context.Context, r ports.Repositories) error {
		_ = r.Jobs.Create(ctx, newJob(t, "n1"))
		_ = r.Queue.Enqueue(ctx, ports.Task{Kind: ports.TaskRender, JobID: "n1", RenderID: "r"})
		return errors.New("rollback")
	})
	start := time.Now()
	_ = l.Wait(context.Background(), 300*time.Millisecond)
	if time.Since(start) < 250*time.Millisecond {
		t.Fatal("rollback não deveria notificar")
	}

	createJob(t, uow, newJob(t, "n2"), ports.Task{Kind: ports.TaskRender, JobID: "n2", RenderID: "r"})
	start = time.Now()
	_ = l.Wait(context.Background(), 5*time.Second)
	if time.Since(start) > 2*time.Second {
		t.Fatal("commit deveria notificar rapidamente")
	}
}

// TestServiceWithPostgres exercita os use cases reais sobre o Postgres.
func TestServiceWithPostgres(t *testing.T) {
	db, _ := setupDB(t)
	svc := application.NewService(application.Deps{
		UoW:       postgres.NewUnitOfWork(db),
		Generator: stubGenerator{},
		Catalog:   acceptAllCatalog{},
		IDs:       &seqIDs{},
		Clock:     clock{},
	})
	ctx := context.Background()
	q := postgres.NewQueue(db)

	job, err := svc.CreateVideo(ctx, application.CreateVideoInput{Prompt: "ofertas", TemplateID: "promo"})
	if err != nil {
		t.Fatal(err)
	}
	task, _ := q.Claim(ctx, ports.TaskGenerateStoryboard, "w")
	if task == nil || task.Task.JobID != job.ID() {
		t.Fatalf("tarefa de roteiro: %+v", task)
	}
	if err := svc.GenerateStoryboard(ctx, job.ID()); err != nil {
		t.Fatal(err)
	}
	_ = q.Complete(ctx, task.ID)

	if _, err := svc.RequestRender(ctx, job.ID(), "idem-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RequestRender(ctx, job.ID(), "idem-1"); err != nil {
		t.Fatal(err)
	}
	render, _ := q.Claim(ctx, ports.TaskRender, "w")
	if render == nil || render.Task.RenderID != "idem-1" {
		t.Fatalf("render: %+v", render)
	}
	if dup, _ := q.Claim(ctx, ports.TaskRender, "w"); dup != nil {
		t.Fatal("idempotency key gerou render duplicado")
	}
	for _, step := range []func() error{
		func() error { _, err := svc.StartRender(ctx, job.ID(), "idem-1"); return err },
		func() error { _, err := svc.ReportRenderProgress(ctx, job.ID(), "idem-1", 0.5); return err },
		func() error { _, err := svc.CompleteRender(ctx, job.ID(), "idem-1", "x/idem-1.mp4"); return err },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	final, _ := svc.GetVideo(ctx, job.ID())
	if final.Status() != domain.StatusCompleted || final.OutputKey() != "x/idem-1.mp4" {
		t.Fatalf("final: %s %s", final.Status(), final.OutputKey())
	}
}

type stubGenerator struct{}

func (stubGenerator) Generate(context.Context, ports.GenerateInput) (ports.GenerateOutput, error) {
	return ports.GenerateOutput{Storyboard: []byte(`{"schemaVersion":1}`), Model: "stub"}, nil
}

type acceptAllCatalog struct{}

func (acceptAllCatalog) Latest(id string) (domain.TemplateRef, bool) {
	return domain.TemplateRef{ID: id, Version: 1}, id == "promo"
}
func (acceptAllCatalog) JSONSchema(domain.TemplateRef) ([]byte, bool) { return []byte(`{}`), true }
func (acceptAllCatalog) Validate(domain.TemplateRef, []byte) ([]ports.ValidationIssue, error) {
	return nil, nil
}

type seqIDs struct {
	mu sync.Mutex
	n  int
}

func (s *seqIDs) NewJobID() domain.JobID {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	return domain.JobID(fmt.Sprintf("job-%d", s.n))
}

func (s *seqIDs) NewRenderID() domain.RenderID {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	return domain.RenderID(fmt.Sprintf("render-%d", s.n))
}

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }
