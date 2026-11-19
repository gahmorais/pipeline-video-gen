package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// --- CreateVideo -------------------------------------------------------------

type CreateVideoInput struct {
	Prompt     string
	TemplateID string
}

// CreateVideo cria o job em DRAFT e enfileira a geração do roteiro na mesma transação.
func (s *Service) CreateVideo(ctx context.Context, in CreateVideoInput) (*domain.VideoJob, error) {
	prompt, err := domain.NewPrompt(in.Prompt)
	if err != nil {
		return nil, err
	}
	ref, ok := s.d.Catalog.Latest(in.TemplateID)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTemplate, in.TemplateID)
	}
	job, err := domain.NewVideoJob(s.d.IDs.NewJobID(), prompt, ref, s.d.Clock.Now())
	if err != nil {
		return nil, err
	}
	var events []domain.Event
	err = s.d.UoW.Do(ctx, func(ctx context.Context, r ports.Repositories) error {
		if err := r.Jobs.Create(ctx, job); err != nil {
			return err
		}
		if err := r.Queue.Enqueue(ctx, ports.Task{Kind: ports.TaskGenerateStoryboard, JobID: job.ID()}); err != nil {
			return fmt.Errorf("enfileirando roteiro: %w", err)
		}
		events = job.PullEvents()
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.publish(ctx, events)
	return job, nil
}

// --- GetVideo ----------------------------------------------------------------

func (s *Service) GetVideo(ctx context.Context, id domain.JobID) (*domain.VideoJob, error) {
	var job *domain.VideoJob
	err := s.d.UoW.Do(ctx, func(ctx context.Context, r ports.Repositories) error {
		var err error
		job, err = r.Jobs.Get(ctx, id)
		return err
	})
	return job, err
}

// --- GenerateStoryboard (storyboard worker) -----------------------------------

// GenerateStoryboard processa uma tarefa de roteiro:
//  1. marca GENERATING_STORYBOARD (curta transação);
//  2. chama o LLM FORA de transação (operação lenta);
//  3. valida contra o schema e anexa — ou marca FAILED com o motivo.
//
// Tarefas obsoletas (job já não aguarda roteiro) são ignoradas sem erro.
func (s *Service) GenerateStoryboard(ctx context.Context, id domain.JobID) error {
	job, err := s.mutate(ctx, id, func(j *domain.VideoJob) (bool, []ports.Task, error) {
		switch j.Status() {
		case domain.StatusDraft:
			return true, nil, j.StartStoryboardGeneration(s.d.Clock.Now())
		case domain.StatusGeneratingStoryboard:
			return false, nil, nil
		default:
			return false, nil, errStaleTask
		}
	})
	if errors.Is(err, errStaleTask) {
		return nil
	}
	if err != nil {
		return err
	}

	schema, ok := s.d.Catalog.JSONSchema(job.Template())
	if !ok {
		return s.failStoryboard(ctx, id, fmt.Sprintf("schema ausente para %s", job.Template()))
	}

	out, genErr := s.d.Generator.Generate(ctx, ports.GenerateInput{
		JobID:      job.ID(),
		Prompt:     job.Prompt(),
		Template:   job.Template(),
		JSONSchema: schema,
	})
	if genErr != nil {
		return s.failStoryboard(ctx, id, "falha ao gerar roteiro: "+genErr.Error())
	}

	sb, err := s.validate(job.Template(), out.Storyboard)
	if err != nil {
		return s.failStoryboard(ctx, id, "roteiro gerado inválido: "+err.Error())
	}

	_, err = s.mutate(ctx, id, func(j *domain.VideoJob) (bool, []ports.Task, error) {
		if j.Status() != domain.StatusGeneratingStoryboard {
			return false, nil, nil // cancelado/alterado enquanto o LLM respondia
		}
		return true, nil, j.AttachStoryboard(sb, s.d.Clock.Now())
	})
	return err
}

var errStaleTask = errors.New("tarefa obsoleta")

func (s *Service) failStoryboard(ctx context.Context, id domain.JobID, reason string) error {
	_, err := s.mutate(ctx, id, func(j *domain.VideoJob) (bool, []ports.Task, error) {
		if j.Status() != domain.StatusGeneratingStoryboard {
			return false, nil, nil
		}
		return true, nil, j.Fail(reason, s.d.Clock.Now())
	})
	return err
}

// --- UpdateStoryboard ----------------------------------------------------------

// UpdateStoryboard aplica edição do usuário (validada), sem chamar o LLM.
func (s *Service) UpdateStoryboard(ctx context.Context, id domain.JobID, raw []byte) (*domain.VideoJob, error) {
	return s.mutate(ctx, id, func(j *domain.VideoJob) (bool, []ports.Task, error) {
		sb, err := s.validate(j.Template(), raw)
		if err != nil {
			return false, nil, err
		}
		return true, nil, j.UpdateStoryboard(sb, s.d.Clock.Now())
	})
}

// --- RequestRender -------------------------------------------------------------

// RequestRender enfileira o render. A idempotency key (opcional) vira o renderId:
// repetir a mesma requisição não cria um segundo render.
func (s *Service) RequestRender(ctx context.Context, id domain.JobID, idempotencyKey string) (*domain.VideoJob, error) {
	renderID := s.d.IDs.NewRenderID()
	if idempotencyKey != "" {
		parsed, err := domain.ParseRenderID(idempotencyKey)
		if err != nil {
			return nil, err
		}
		renderID = parsed
	}
	return s.mutate(ctx, id, func(j *domain.VideoJob) (bool, []ports.Task, error) {
		changed, err := j.RequestRender(renderID, s.d.Clock.Now())
		if err != nil || !changed {
			return false, nil, err
		}
		return true, []ports.Task{{Kind: ports.TaskRender, JobID: j.ID(), RenderID: renderID}}, nil
	})
}

// --- Comandos reportados pelo render worker --------------------------------------

func (s *Service) StartRender(ctx context.Context, id domain.JobID, renderID domain.RenderID) (*domain.VideoJob, error) {
	return s.mutate(ctx, id, func(j *domain.VideoJob) (bool, []ports.Task, error) {
		before := j.Status()
		if err := j.StartRendering(renderID, s.d.Clock.Now()); err != nil {
			return false, nil, err
		}
		return before != j.Status(), nil, nil
	})
}

func (s *Service) ReportRenderProgress(ctx context.Context, id domain.JobID, renderID domain.RenderID, progress float64) (*domain.VideoJob, error) {
	return s.mutate(ctx, id, func(j *domain.VideoJob) (bool, []ports.Task, error) {
		changed, err := j.ReportRenderProgress(renderID, progress, s.d.Clock.Now())
		return changed, nil, err
	})
}

func (s *Service) CompleteRender(ctx context.Context, id domain.JobID, renderID domain.RenderID, outputKey string) (*domain.VideoJob, error) {
	return s.mutate(ctx, id, func(j *domain.VideoJob) (bool, []ports.Task, error) {
		before := j.Status()
		if err := j.CompleteRender(renderID, outputKey, s.d.Clock.Now()); err != nil {
			return false, nil, err
		}
		return before != j.Status(), nil, nil
	})
}

// FailRender marca o job como FAILED se o renderId ainda for o atual.
func (s *Service) FailRender(ctx context.Context, id domain.JobID, renderID domain.RenderID, reason string) (*domain.VideoJob, error) {
	return s.mutate(ctx, id, func(j *domain.VideoJob) (bool, []ports.Task, error) {
		if j.RenderID() != renderID {
			return false, nil, fmt.Errorf("%w: recebido %q, atual %q", domain.ErrRenderMismatch, renderID, j.RenderID())
		}
		return true, nil, j.Fail(reason, s.d.Clock.Now())
	})
}

// --- RetryJob ----------------------------------------------------------------

// RetryJob retoma um job em FAILED e reenfileira a tarefa correspondente.
func (s *Service) RetryJob(ctx context.Context, id domain.JobID) (*domain.VideoJob, error) {
	return s.mutate(ctx, id, func(j *domain.VideoJob) (bool, []ports.Task, error) {
		if err := j.Retry(s.d.Clock.Now()); err != nil {
			return false, nil, err
		}
		switch j.Status() {
		case domain.StatusGeneratingStoryboard:
			return true, []ports.Task{{Kind: ports.TaskGenerateStoryboard, JobID: j.ID()}}, nil
		case domain.StatusRenderQueued:
			return true, []ports.Task{{Kind: ports.TaskRender, JobID: j.ID(), RenderID: j.RenderID()}}, nil
		default:
			return false, nil, fmt.Errorf("estado inesperado após retry: %s", j.Status())
		}
	})
}
