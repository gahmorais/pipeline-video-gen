package application_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/application"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

var (
	now        = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	validSB    = []byte(`{"valid":true,"scenes":[]}`)
	invalidSB  = []byte(`{"valid":false}`)
	ctx        = context.Background()
	templateID = "promo"
)

type harness struct {
	store  *memStore
	gen    *fakeGenerator
	events *recordingPublisher
	svc    *application.Service
}

func newHarness() *harness {
	h := &harness{
		store:  newMemStore(),
		gen:    &fakeGenerator{out: ports.GenerateOutput{Storyboard: validSB, Model: "fake"}},
		events: &recordingPublisher{},
	}
	h.svc = application.NewService(application.Deps{
		UoW:       h.store,
		Generator: h.gen,
		Catalog:   fakeCatalog{},
		Events:    h.events,
		IDs:       &seqIDs{},
		Clock:     fixedClock{now},
	})
	return h
}

// readyJob cria um job e gera o roteiro (STORYBOARD_READY).
func (h *harness) readyJob(t *testing.T) domain.JobID {
	t.Helper()
	job, err := h.svc.CreateVideo(ctx, application.CreateVideoInput{Prompt: "promo de leitores", TemplateID: templateID})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.GenerateStoryboard(ctx, job.ID()); err != nil {
		t.Fatal(err)
	}
	return job.ID()
}

func (h *harness) status(id domain.JobID) domain.JobStatus { return h.store.snapshot(id).Status }

func TestCreateVideo(t *testing.T) {
	h := newHarness()
	job, err := h.svc.CreateVideo(ctx, application.CreateVideoInput{Prompt: "  ofertas  ", TemplateID: templateID})
	if err != nil {
		t.Fatal(err)
	}
	snap := h.store.snapshot(job.ID())
	if snap.Status != domain.StatusDraft || snap.Prompt != "ofertas" || snap.Template.String() != "promo@1" || snap.Version != 1 {
		t.Fatalf("job persistido incorreto: %+v", snap)
	}
	if got := h.store.tasks(); !slices.Equal(got, []ports.Task{{Kind: ports.TaskGenerateStoryboard, JobID: job.ID()}}) {
		t.Fatalf("fila: %+v", got)
	}
	if !slices.Equal(h.events.names(), []string{"job.status_changed"}) {
		t.Fatalf("eventos: %v", h.events.names())
	}
}

func TestCreateVideoValidation(t *testing.T) {
	tests := []struct {
		name    string
		in      application.CreateVideoInput
		wantErr error
	}{
		{"prompt vazio", application.CreateVideoInput{Prompt: " ", TemplateID: templateID}, domain.ErrInvalidPrompt},
		{"template desconhecido", application.CreateVideoInput{Prompt: "x", TemplateID: "nope"}, application.ErrUnknownTemplate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness()
			if _, err := h.svc.CreateVideo(ctx, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("esperado %v, obtido %v", tt.wantErr, err)
			}
			if len(h.store.tasks()) != 0 {
				t.Fatal("nada deve ser enfileirado")
			}
		})
	}
}

func TestCreateVideoIsAtomicWithQueue(t *testing.T) {
	h := newHarness()
	h.store.failEnqueue = errors.New("fila indisponível")
	if _, err := h.svc.CreateVideo(ctx, application.CreateVideoInput{Prompt: "x", TemplateID: templateID}); err == nil {
		t.Fatal("esperado erro")
	}
	if n := len(h.store.jobs); n != 0 {
		t.Fatalf("job não deve ser persistido sem a tarefa na fila (jobs=%d)", n)
	}
	if len(h.events.names()) != 0 {
		t.Fatal("nenhum evento deve ser publicado em transação desfeita")
	}
}

func TestGenerateStoryboardSuccess(t *testing.T) {
	h := newHarness()
	id := h.readyJob(t)
	snap := h.store.snapshot(id)
	if snap.Status != domain.StatusStoryboardReady || string(snap.Storyboard) != string(validSB) {
		t.Fatalf("job: %+v", snap)
	}
	call := h.gen.calls[0]
	if call.Prompt != "promo de leitores" || call.Template.String() != "promo@1" || len(call.JSONSchema) == 0 {
		t.Fatalf("entrada do LLM incorreta: %+v", call)
	}
}

func TestGenerateStoryboardFailures(t *testing.T) {
	tests := []struct {
		name       string
		gen        fakeGenerator
		wantReason string
	}{
		{"erro do LLM", fakeGenerator{err: errors.New("503")}, "falha ao gerar roteiro"},
		{"saída fora do schema", fakeGenerator{out: ports.GenerateOutput{Storyboard: invalidSB}}, "roteiro gerado inválido"},
		{"saída não-JSON", fakeGenerator{out: ports.GenerateOutput{Storyboard: []byte("oops")}}, "roteiro gerado inválido"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness()
			*h.gen = tt.gen
			job, _ := h.svc.CreateVideo(ctx, application.CreateVideoInput{Prompt: "x", TemplateID: templateID})
			if err := h.svc.GenerateStoryboard(ctx, job.ID()); err != nil {
				t.Fatalf("falha do LLM deve virar FAILED, não erro: %v", err)
			}
			snap := h.store.snapshot(job.ID())
			if snap.Status != domain.StatusFailed || !strings.Contains(snap.Failure.Reason, tt.wantReason) {
				t.Fatalf("esperado FAILED com %q, obtido %s %+v", tt.wantReason, snap.Status, snap.Failure)
			}
			if snap.Failure.From != domain.StatusGeneratingStoryboard {
				t.Fatalf("origem da falha: %s", snap.Failure.From)
			}
		})
	}
}

func TestGenerateStoryboardIgnoresStaleTasks(t *testing.T) {
	h := newHarness()
	id := h.readyJob(t)
	calls := len(h.gen.calls)
	if err := h.svc.GenerateStoryboard(ctx, id); err != nil {
		t.Fatal(err)
	}
	if len(h.gen.calls) != calls {
		t.Fatal("tarefa obsoleta não deve chamar o LLM")
	}
}

func TestGenerateStoryboardNotFound(t *testing.T) {
	h := newHarness()
	if err := h.svc.GenerateStoryboard(ctx, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("esperado ErrNotFound, obtido %v", err)
	}
}

func TestGenerateStoryboardDoesNotOverwriteConcurrentChange(t *testing.T) {
	h := newHarness()
	job, _ := h.svc.CreateVideo(ctx, application.CreateVideoInput{Prompt: "x", TemplateID: templateID})
	// Enquanto o LLM responde, o job falha por outro motivo (ex.: timeout de supervisão).
	h.gen.during = func() {
		_ = h.store.Do(ctx, func(ctx context.Context, r ports.Repositories) error {
			j, _ := r.Jobs.Get(ctx, job.ID())
			_ = j.Fail("cancelado", now)
			return r.Jobs.Update(ctx, j)
		})
	}
	if err := h.svc.GenerateStoryboard(ctx, job.ID()); err != nil {
		t.Fatal(err)
	}
	snap := h.store.snapshot(job.ID())
	if snap.Status != domain.StatusFailed || snap.Failure.Reason != "cancelado" {
		t.Fatalf("resultado do LLM não deve sobrescrever mudança concorrente: %+v", snap)
	}
}

func TestUpdateStoryboard(t *testing.T) {
	h := newHarness()
	id := h.readyJob(t)

	edited := []byte(`{"valid":true,"scenes":[1]}`)
	if _, err := h.svc.UpdateStoryboard(ctx, id, edited); err != nil {
		t.Fatal(err)
	}
	if got := string(h.store.snapshot(id).Storyboard); got != string(edited) {
		t.Fatalf("storyboard: %s", got)
	}

	var verr *application.ValidationError
	if _, err := h.svc.UpdateStoryboard(ctx, id, invalidSB); !errors.As(err, &verr) || len(verr.Issues) == 0 {
		t.Fatalf("esperado ValidationError com issues, obtido %v", err)
	}
	if _, err := h.svc.UpdateStoryboard(ctx, id, []byte(`[1]`)); !errors.Is(err, domain.ErrInvalidStoryboard) {
		t.Fatalf("esperado ErrInvalidStoryboard, obtido %v", err)
	}
	if got := string(h.store.snapshot(id).Storyboard); got != string(edited) {
		t.Fatal("edição inválida não deve alterar o storyboard")
	}
	if len(h.gen.calls) != 1 {
		t.Fatal("edição não deve chamar o LLM")
	}
}

func TestRequestRenderIdempotencyKey(t *testing.T) {
	h := newHarness()
	id := h.readyJob(t)

	for i := 0; i < 3; i++ {
		job, err := h.svc.RequestRender(ctx, id, "idem-123")
		if err != nil {
			t.Fatal(err)
		}
		if job.RenderID() != "idem-123" || job.Status() != domain.StatusRenderQueued {
			t.Fatalf("job: %s %s", job.RenderID(), job.Status())
		}
	}
	renders := 0
	for _, task := range h.store.tasks() {
		if task.Kind == ports.TaskRender {
			renders++
			if task.RenderID != "idem-123" {
				t.Fatalf("renderId da tarefa: %s", task.RenderID)
			}
		}
	}
	if renders != 1 {
		t.Fatalf("esperado 1 tarefa de render, obtido %d", renders)
	}
}

func TestRequestRenderRejections(t *testing.T) {
	h := newHarness()
	job, _ := h.svc.CreateVideo(ctx, application.CreateVideoInput{Prompt: "x", TemplateID: templateID})
	if _, err := h.svc.RequestRender(ctx, job.ID(), ""); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("render sem roteiro deve falhar: %v", err)
	}
	if _, err := h.svc.RequestRender(ctx, job.ID(), "../x"); !errors.Is(err, domain.ErrInvalidID) {
		t.Fatalf("idempotency key insegura deve falhar: %v", err)
	}
}

func TestRenderLifecycle(t *testing.T) {
	h := newHarness()
	id := h.readyJob(t)
	job, err := h.svc.RequestRender(ctx, id, "")
	if err != nil {
		t.Fatal(err)
	}
	rid := job.RenderID()

	steps := []struct {
		name string
		run  func() error
		want domain.JobStatus
	}{
		{"start", func() error { _, err := h.svc.StartRender(ctx, id, rid); return err }, domain.StatusRendering},
		{"start repetido", func() error { _, err := h.svc.StartRender(ctx, id, rid); return err }, domain.StatusRendering},
		{"progresso", func() error { _, err := h.svc.ReportRenderProgress(ctx, id, rid, 0.5); return err }, domain.StatusRendering},
		{"progresso regressivo ignorado", func() error { _, err := h.svc.ReportRenderProgress(ctx, id, rid, 0.2); return err }, domain.StatusRendering},
		{"complete", func() error { _, err := h.svc.CompleteRender(ctx, id, rid, "out.mp4"); return err }, domain.StatusCompleted},
		{"complete repetido", func() error { _, err := h.svc.CompleteRender(ctx, id, rid, "out.mp4"); return err }, domain.StatusCompleted},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if got := h.status(id); got != s.want {
			t.Fatalf("%s: status %s, esperado %s", s.name, got, s.want)
		}
	}
	snap := h.store.snapshot(id)
	if snap.OutputKey != "out.mp4" || snap.RenderProgress != 1 {
		t.Fatalf("saída: %+v", snap)
	}
	// Repetições idempotentes não devem gerar escrita extra (versão estável).
	v := snap.Version
	_, _ = h.svc.CompleteRender(ctx, id, rid, "out.mp4")
	if h.store.snapshot(id).Version != v {
		t.Fatal("comando idempotente não deve incrementar a versão")
	}
	if !slices.Contains(h.events.names(), "job.render_progressed") {
		t.Fatalf("evento de progresso esperado: %v", h.events.names())
	}
}

func TestStaleWorkerIsRejected(t *testing.T) {
	h := newHarness()
	id := h.readyJob(t)
	_, _ = h.svc.RequestRender(ctx, id, "atual")
	if _, err := h.svc.StartRender(ctx, id, "velho"); !errors.Is(err, domain.ErrRenderMismatch) {
		t.Fatalf("esperado ErrRenderMismatch, obtido %v", err)
	}
	if _, err := h.svc.FailRender(ctx, id, "velho", "x"); !errors.Is(err, domain.ErrRenderMismatch) {
		t.Fatalf("esperado ErrRenderMismatch, obtido %v", err)
	}
	if h.status(id) != domain.StatusRenderQueued {
		t.Fatal("status não deve mudar")
	}
}

func TestFailRenderAndRetryRequeues(t *testing.T) {
	h := newHarness()
	id := h.readyJob(t)
	job, _ := h.svc.RequestRender(ctx, id, "")
	rid := job.RenderID()
	_, _ = h.svc.StartRender(ctx, id, rid)

	if _, err := h.svc.FailRender(ctx, id, rid, "chrome crashed"); err != nil {
		t.Fatal(err)
	}
	if h.status(id) != domain.StatusFailed {
		t.Fatalf("status %s", h.status(id))
	}

	before := len(h.store.tasks())
	if _, err := h.svc.RetryJob(ctx, id); err != nil {
		t.Fatal(err)
	}
	tasks := h.store.tasks()
	if len(tasks) != before+1 || tasks[len(tasks)-1] != (ports.Task{Kind: ports.TaskRender, JobID: id, RenderID: rid}) {
		t.Fatalf("retry deve reenfileirar o render: %+v", tasks)
	}
	if h.status(id) != domain.StatusRenderQueued {
		t.Fatalf("status %s", h.status(id))
	}
}

func TestRetryStoryboardRequeues(t *testing.T) {
	h := newHarness()
	h.gen.err = errors.New("timeout")
	job, _ := h.svc.CreateVideo(ctx, application.CreateVideoInput{Prompt: "x", TemplateID: templateID})
	_ = h.svc.GenerateStoryboard(ctx, job.ID())

	if _, err := h.svc.RetryJob(ctx, job.ID()); err != nil {
		t.Fatal(err)
	}
	tasks := h.store.tasks()
	if last := tasks[len(tasks)-1]; last.Kind != ports.TaskGenerateStoryboard {
		t.Fatalf("esperado tarefa de roteiro, obtido %+v", last)
	}

	// Após retry, o worker processa a partir de GENERATING_STORYBOARD.
	h.gen.err = nil
	if err := h.svc.GenerateStoryboard(ctx, job.ID()); err != nil {
		t.Fatal(err)
	}
	if h.status(job.ID()) != domain.StatusStoryboardReady {
		t.Fatalf("status %s", h.status(job.ID()))
	}
}

func TestRetryNotAllowedOutsideFailed(t *testing.T) {
	h := newHarness()
	id := h.readyJob(t)
	if _, err := h.svc.RetryJob(ctx, id); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("esperado ErrInvalidTransition, obtido %v", err)
	}
}

func TestGetVideo(t *testing.T) {
	h := newHarness()
	id := h.readyJob(t)
	job, err := h.svc.GetVideo(ctx, id)
	if err != nil || job.ID() != id {
		t.Fatalf("GetVideo: %v %v", job, err)
	}
	if _, err := h.svc.GetVideo(ctx, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("esperado ErrNotFound, obtido %v", err)
	}
}
