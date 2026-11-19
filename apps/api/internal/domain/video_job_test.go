package domain_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
)

var t0 = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

const renderA domain.RenderID = "render-a"

func mustStoryboard(t *testing.T) domain.Storyboard {
	t.Helper()
	sb, err := domain.NewStoryboard([]byte(`{"schemaVersion":1}`))
	if err != nil {
		t.Fatal(err)
	}
	return sb
}

func newJob(t *testing.T) *domain.VideoJob {
	t.Helper()
	tpl, _ := domain.NewTemplateRef("promo", 1)
	j, err := domain.NewVideoJob("job-1", "um vídeo de promoção", tpl, t0)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

// jobIn leva um job até o status pedido usando apenas transições válidas.
func jobIn(t *testing.T, status domain.JobStatus) *domain.VideoJob {
	t.Helper()
	j := newJob(t)
	steps := map[domain.JobStatus][]func() error{
		domain.StatusDraft:                {},
		domain.StatusGeneratingStoryboard: {func() error { return j.StartStoryboardGeneration(t0) }},
		domain.StatusStoryboardReady: {
			func() error { return j.StartStoryboardGeneration(t0) },
			func() error { return j.AttachStoryboard(mustStoryboard(t), t0) },
		},
		domain.StatusRenderQueued: {
			func() error { return j.StartStoryboardGeneration(t0) },
			func() error { return j.AttachStoryboard(mustStoryboard(t), t0) },
			func() error { _, err := j.RequestRender(renderA, t0); return err },
		},
		domain.StatusRendering: {
			func() error { return j.StartStoryboardGeneration(t0) },
			func() error { return j.AttachStoryboard(mustStoryboard(t), t0) },
			func() error { _, err := j.RequestRender(renderA, t0); return err },
			func() error { return j.StartRendering(renderA, t0) },
		},
		domain.StatusCompleted: {
			func() error { return j.StartStoryboardGeneration(t0) },
			func() error { return j.AttachStoryboard(mustStoryboard(t), t0) },
			func() error { _, err := j.RequestRender(renderA, t0); return err },
			func() error { return j.StartRendering(renderA, t0) },
			func() error { return j.CompleteRender(renderA, "job-1/render-a.mp4", t0) },
		},
		domain.StatusFailed: {
			func() error { return j.StartStoryboardGeneration(t0) },
			func() error { return j.Fail("llm indisponível", t0) },
		},
	}
	for i, step := range steps[status] {
		if err := step(); err != nil {
			t.Fatalf("preparando %s, passo %d: %v", status, i, err)
		}
	}
	if j.Status() != status {
		t.Fatalf("esperado %s, obtido %s", status, j.Status())
	}
	j.PullEvents()
	return j
}

// TestTransitionMatrix verifica todas as ações em todos os estados:
// estados fora de `allowed` devem falhar com ErrInvalidTransition sem alterar o job.
func TestTransitionMatrix(t *testing.T) {
	type action struct {
		name    string
		run     func(t *testing.T, j *domain.VideoJob) error
		allowed map[domain.JobStatus]domain.JobStatus // origem → destino
	}

	const renderB domain.RenderID = "render-b"

	actions := []action{
		{
			name: "StartStoryboardGeneration",
			run:  func(_ *testing.T, j *domain.VideoJob) error { return j.StartStoryboardGeneration(t0) },
			allowed: map[domain.JobStatus]domain.JobStatus{
				domain.StatusDraft: domain.StatusGeneratingStoryboard,
			},
		},
		{
			name: "AttachStoryboard",
			run:  func(t *testing.T, j *domain.VideoJob) error { return j.AttachStoryboard(mustStoryboard(t), t0) },
			allowed: map[domain.JobStatus]domain.JobStatus{
				domain.StatusGeneratingStoryboard: domain.StatusStoryboardReady,
			},
		},
		{
			name: "UpdateStoryboard",
			run:  func(t *testing.T, j *domain.VideoJob) error { return j.UpdateStoryboard(mustStoryboard(t), t0) },
			allowed: map[domain.JobStatus]domain.JobStatus{
				domain.StatusStoryboardReady: domain.StatusStoryboardReady,
				domain.StatusCompleted:       domain.StatusStoryboardReady,
			},
		},
		{
			name: "RequestRender(novo renderID)",
			run: func(_ *testing.T, j *domain.VideoJob) error {
				_, err := j.RequestRender(renderB, t0)
				return err
			},
			allowed: map[domain.JobStatus]domain.JobStatus{
				domain.StatusStoryboardReady: domain.StatusRenderQueued,
				domain.StatusCompleted:       domain.StatusRenderQueued,
			},
		},
		{
			name: "StartRendering",
			run:  func(_ *testing.T, j *domain.VideoJob) error { return j.StartRendering(renderA, t0) },
			allowed: map[domain.JobStatus]domain.JobStatus{
				domain.StatusRenderQueued: domain.StatusRendering,
				domain.StatusRendering:    domain.StatusRendering, // reentrega idempotente
			},
		},
		{
			name: "CompleteRender",
			run: func(_ *testing.T, j *domain.VideoJob) error {
				return j.CompleteRender(renderA, "job-1/render-a.mp4", t0)
			},
			allowed: map[domain.JobStatus]domain.JobStatus{
				domain.StatusRendering: domain.StatusCompleted,
				domain.StatusCompleted: domain.StatusCompleted, // idempotente
			},
		},
		{
			name: "Fail",
			run:  func(_ *testing.T, j *domain.VideoJob) error { return j.Fail("boom", t0) },
			allowed: map[domain.JobStatus]domain.JobStatus{
				domain.StatusDraft:                domain.StatusFailed,
				domain.StatusGeneratingStoryboard: domain.StatusFailed,
				domain.StatusRenderQueued:         domain.StatusFailed,
				domain.StatusRendering:            domain.StatusFailed,
			},
		},
		{
			name: "Retry",
			run:  func(_ *testing.T, j *domain.VideoJob) error { return j.Retry(t0) },
			allowed: map[domain.JobStatus]domain.JobStatus{
				domain.StatusFailed: domain.StatusGeneratingStoryboard,
			},
		},
	}

	for _, a := range actions {
		for _, from := range domain.AllStatuses {
			t.Run(a.name+"/"+string(from), func(t *testing.T) {
				j := jobIn(t, from)
				before := j.Snapshot()
				err := a.run(t, j)

				to, ok := a.allowed[from]
				if !ok {
					// Ações de render em estados sem render ativo falham por renderID, não por transição.
					if !errors.Is(err, domain.ErrInvalidTransition) && !errors.Is(err, domain.ErrRenderMismatch) {
						t.Fatalf("esperado erro de transição, obtido %v", err)
					}
					if j.Status() != before.Status || j.Version() != before.Version {
						t.Fatalf("job alterado após transição inválida")
					}
					if len(j.PullEvents()) != 0 {
						t.Fatalf("transição inválida não deve emitir eventos")
					}
					return
				}
				if err != nil {
					t.Fatalf("transição %s→%s deveria ser válida: %v", from, to, err)
				}
				if j.Status() != to {
					t.Fatalf("esperado %s, obtido %s", to, j.Status())
				}
			})
		}
	}
}

func TestTransitionErrorDetails(t *testing.T) {
	j := jobIn(t, domain.StatusDraft)
	err := j.AttachStoryboard(mustStoryboard(t), t0)
	var te *domain.TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("esperado *TransitionError, obtido %T", err)
	}
	if te.From != domain.StatusDraft || te.Action != "attach_storyboard" {
		t.Fatalf("detalhes incorretos: %+v", te)
	}
}

func TestStatusChangedEvents(t *testing.T) {
	j := newJob(t)
	events := j.PullEvents()
	if len(events) != 1 {
		t.Fatalf("esperado 1 evento de criação, obtido %d", len(events))
	}
	if sc, ok := events[0].(domain.StatusChanged); !ok || sc.To != domain.StatusDraft {
		t.Fatalf("evento inicial incorreto: %#v", events[0])
	}

	_ = j.StartStoryboardGeneration(t0)
	_ = j.AttachStoryboard(mustStoryboard(t), t0.Add(time.Second))
	events = j.PullEvents()
	names := make([]string, 0, len(events))
	for _, e := range events {
		names = append(names, e.EventName())
	}
	want := []string{"job.status_changed", "job.storyboard_updated", "job.status_changed"}
	if len(names) != len(want) {
		t.Fatalf("eventos: %v, esperado %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("eventos: %v, esperado %v", names, want)
		}
	}
	if len(j.PullEvents()) != 0 {
		t.Fatal("PullEvents deve limpar a lista")
	}
	if !j.UpdatedAt().Equal(t0.Add(time.Second)) {
		t.Fatalf("updatedAt não avançou: %v", j.UpdatedAt())
	}
}

func TestRequestRenderIdempotency(t *testing.T) {
	tests := []struct {
		name        string
		status      domain.JobStatus
		renderID    domain.RenderID
		wantChanged bool
		wantErr     error
	}{
		{"mesmo renderID na fila", domain.StatusRenderQueued, renderA, false, nil},
		{"mesmo renderID renderizando", domain.StatusRendering, renderA, false, nil},
		{"mesmo renderID concluído", domain.StatusCompleted, renderA, false, nil},
		{"outro renderID durante render", domain.StatusRendering, "render-x", false, domain.ErrInvalidTransition},
		{"novo render após concluir", domain.StatusCompleted, "render-x", true, nil},
		{"renderID vazio", domain.StatusStoryboardReady, "", false, domain.ErrInvalidID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := jobIn(t, tt.status)
			changed, err := j.RequestRender(tt.renderID, t0)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("erro: obtido %v, esperado %v", err, tt.wantErr)
			}
			if changed != tt.wantChanged {
				t.Fatalf("changed: obtido %v, esperado %v", changed, tt.wantChanged)
			}
		})
	}
}

func TestRenderMismatchProtectsAgainstStaleWorkers(t *testing.T) {
	j := jobIn(t, domain.StatusRendering)
	if err := j.CompleteRender("render-velho", "x.mp4", t0); !errors.Is(err, domain.ErrRenderMismatch) {
		t.Fatalf("esperado ErrRenderMismatch, obtido %v", err)
	}
	if _, err := j.ReportRenderProgress("render-velho", 0.5, t0); !errors.Is(err, domain.ErrRenderMismatch) {
		t.Fatalf("esperado ErrRenderMismatch, obtido %v", err)
	}
	if j.Status() != domain.StatusRendering {
		t.Fatal("status não deve mudar")
	}
}

func TestReportRenderProgress(t *testing.T) {
	tests := []struct {
		name        string
		sequence    []float64
		wantFinal   float64
		wantEvents  int
		wantErrLast error
	}{
		{"avança monotonicamente", []float64{0.1, 0.5, 0.9}, 0.9, 3, nil},
		{"ignora duplicados e regressões", []float64{0.5, 0.5, 0.3}, 0.5, 1, nil},
		{"rejeita acima de 1", []float64{1.5}, 0, 0, domain.ErrInvalidProgress},
		{"rejeita negativo", []float64{-0.1}, 0, 0, domain.ErrInvalidProgress},
		{"rejeita NaN", []float64{math.NaN()}, 0, 0, domain.ErrInvalidProgress},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := jobIn(t, domain.StatusRendering)
			var err error
			for _, p := range tt.sequence {
				_, err = j.ReportRenderProgress(renderA, p, t0)
			}
			if !errors.Is(err, tt.wantErrLast) {
				t.Fatalf("erro: obtido %v, esperado %v", err, tt.wantErrLast)
			}
			if j.RenderProgress() != tt.wantFinal {
				t.Fatalf("progresso: obtido %v, esperado %v", j.RenderProgress(), tt.wantFinal)
			}
			if got := len(j.PullEvents()); got != tt.wantEvents {
				t.Fatalf("eventos: obtido %d, esperado %d", got, tt.wantEvents)
			}
		})
	}
}

func TestCompleteRenderSetsOutput(t *testing.T) {
	j := jobIn(t, domain.StatusRendering)
	if err := j.CompleteRender(renderA, "job-1/render-a.mp4", t0); err != nil {
		t.Fatal(err)
	}
	if j.OutputKey() != "job-1/render-a.mp4" || j.RenderProgress() != 1 {
		t.Fatalf("saída incorreta: key=%q progress=%v", j.OutputKey(), j.RenderProgress())
	}
	if err := j.CompleteRender(renderA, "", t0); !errors.Is(err, domain.ErrInvalidID) {
		t.Fatalf("output key vazio deve falhar: %v", err)
	}
}

func TestUpdateStoryboardAfterCompletedResetsRender(t *testing.T) {
	j := jobIn(t, domain.StatusCompleted)
	if err := j.UpdateStoryboard(mustStoryboard(t), t0); err != nil {
		t.Fatal(err)
	}
	if j.RenderID() != "" || j.OutputKey() != "" || j.RenderProgress() != 0 {
		t.Fatal("render anterior deve ser descartado ao editar")
	}
}

func TestRetry(t *testing.T) {
	tests := []struct {
		name   string
		failIn domain.JobStatus
		want   domain.JobStatus
	}{
		{"falha no draft volta a gerar roteiro", domain.StatusDraft, domain.StatusGeneratingStoryboard},
		{"falha no roteiro volta a gerar roteiro", domain.StatusGeneratingStoryboard, domain.StatusGeneratingStoryboard},
		{"falha na fila de render volta à fila", domain.StatusRenderQueued, domain.StatusRenderQueued},
		{"falha durante render volta à fila", domain.StatusRendering, domain.StatusRenderQueued},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := jobIn(t, tt.failIn)
			if err := j.Fail("erro", t0); err != nil {
				t.Fatal(err)
			}
			if f := j.Failure(); f == nil || f.From != tt.failIn || f.Reason != "erro" {
				t.Fatalf("falha não registrada corretamente: %+v", f)
			}
			if err := j.Retry(t0); err != nil {
				t.Fatal(err)
			}
			if j.Status() != tt.want || j.Failure() != nil || j.RetryCount() != 1 {
				t.Fatalf("estado após retry: status=%s failure=%v retries=%d", j.Status(), j.Failure(), j.RetryCount())
			}
		})
	}
}

func TestRetryLimit(t *testing.T) {
	j := jobIn(t, domain.StatusGeneratingStoryboard)
	for i := 0; i < domain.MaxRetries; i++ {
		if err := j.Fail("erro", t0); err != nil {
			t.Fatal(err)
		}
		if err := j.Retry(t0); err != nil {
			t.Fatalf("retry %d: %v", i+1, err)
		}
	}
	_ = j.Fail("erro", t0)
	if err := j.Retry(t0); !errors.Is(err, domain.ErrRetryLimit) {
		t.Fatalf("esperado ErrRetryLimit, obtido %v", err)
	}
	if j.Status() != domain.StatusFailed {
		t.Fatal("job deve permanecer em FAILED")
	}
}

func TestFailUsesDefaultReason(t *testing.T) {
	j := jobIn(t, domain.StatusRendering)
	_ = j.Fail("", t0)
	if j.Failure().Reason == "" {
		t.Fatal("motivo padrão esperado")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	for _, status := range domain.AllStatuses {
		t.Run(string(status), func(t *testing.T) {
			j := jobIn(t, status)
			j.SetVersion(7)
			restored, err := domain.Rehydrate(j.Snapshot())
			if err != nil {
				t.Fatal(err)
			}
			a, b := j.Snapshot(), restored.Snapshot()
			if a.Status != b.Status || a.RenderID != b.RenderID || a.OutputKey != b.OutputKey ||
				a.Version != b.Version || string(a.Storyboard) != string(b.Storyboard) ||
				(a.Failure == nil) != (b.Failure == nil) || a.RetryCount != b.RetryCount {
				t.Fatalf("snapshot divergente:\n%+v\n%+v", a, b)
			}
			if len(restored.PullEvents()) != 0 {
				t.Fatal("reidratação não deve emitir eventos")
			}
		})
	}
}

func TestRehydrateRejectsCorruptState(t *testing.T) {
	tests := []struct {
		name string
		snap domain.Snapshot
	}{
		{"sem id", domain.Snapshot{Status: domain.StatusDraft}},
		{"status desconhecido", domain.Snapshot{ID: "j", Status: "WAT"}},
		{"failed sem falha", domain.Snapshot{ID: "j", Status: domain.StatusFailed}},
		{"storyboard inválido", domain.Snapshot{ID: "j", Status: domain.StatusStoryboardReady, Storyboard: []byte("[1]")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := domain.Rehydrate(tt.snap); err == nil {
				t.Fatal("esperado erro")
			}
		})
	}
}

func TestSnapshotDoesNotLeakMutableState(t *testing.T) {
	j := jobIn(t, domain.StatusStoryboardReady)
	snap := j.Snapshot()
	snap.Storyboard[0] = 'X'
	if string(j.Storyboard().JSON())[0] != '{' {
		t.Fatal("snapshot expôs o buffer interno do storyboard")
	}
}
