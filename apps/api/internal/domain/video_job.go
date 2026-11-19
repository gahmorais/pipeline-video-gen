package domain

import (
	"fmt"
	"time"
)

// MaxRetries limita quantas vezes um job em FAILED pode ser retomado.
const MaxRetries = 3

// Failure registra o motivo e o estado em que a falha ocorreu.
type Failure struct {
	Reason string
	From   JobStatus
	At     time.Time
}

// VideoJob é o agregado raiz. Todas as mudanças de estado passam por seus métodos,
// que validam a transição e registram eventos de domínio.
//
// Máquina de estados:
//
//	DRAFT → GENERATING_STORYBOARD → STORYBOARD_READY ⇄ (edição)
//	STORYBOARD_READY|COMPLETED → RENDER_QUEUED → RENDERING → COMPLETED
//	DRAFT|GENERATING_STORYBOARD|RENDER_QUEUED|RENDERING → FAILED
//	FAILED → (retry) GENERATING_STORYBOARD | RENDER_QUEUED
type VideoJob struct {
	id             JobID
	prompt         Prompt
	template       TemplateRef
	status         JobStatus
	storyboard     Storyboard
	renderID       RenderID
	renderProgress float64
	outputKey      string
	failure        *Failure
	retryCount     int
	version        int
	createdAt      time.Time
	updatedAt      time.Time

	events []Event
}

// NewVideoJob cria um job em DRAFT.
func NewVideoJob(id JobID, prompt Prompt, template TemplateRef, now time.Time) (*VideoJob, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: job id vazio", ErrInvalidID)
	}
	if prompt == "" {
		return nil, fmt.Errorf("%w: vazio", ErrInvalidPrompt)
	}
	if template.ID == "" || template.Version <= 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTemplate, template)
	}
	j := &VideoJob{
		id:        id,
		prompt:    prompt,
		template:  template,
		status:    StatusDraft,
		createdAt: now,
		updatedAt: now,
	}
	j.record(StatusChanged{baseEvent: baseEvent{id, now}, From: "", To: StatusDraft})
	return j, nil
}

// --- Comandos ---------------------------------------------------------------

// StartStoryboardGeneration: DRAFT → GENERATING_STORYBOARD.
func (j *VideoJob) StartStoryboardGeneration(now time.Time) error {
	if j.status != StatusDraft {
		return j.reject("start_storyboard_generation")
	}
	j.transition(StatusGeneratingStoryboard, now, "")
	return nil
}

// AttachStoryboard: GENERATING_STORYBOARD → STORYBOARD_READY.
func (j *VideoJob) AttachStoryboard(sb Storyboard, now time.Time) error {
	if j.status != StatusGeneratingStoryboard {
		return j.reject("attach_storyboard")
	}
	if sb.IsZero() {
		return fmt.Errorf("%w: vazio", ErrInvalidStoryboard)
	}
	j.storyboard = sb
	j.record(StoryboardUpdated{baseEvent{j.id, now}})
	j.transition(StatusStoryboardReady, now, "")
	return nil
}

// UpdateStoryboard edita o roteiro sem chamar o LLM.
// Permitido em STORYBOARD_READY e em COMPLETED (para re-render); em COMPLETED
// volta para STORYBOARD_READY e descarta a referência ao arquivo antigo.
func (j *VideoJob) UpdateStoryboard(sb Storyboard, now time.Time) error {
	if j.status != StatusStoryboardReady && j.status != StatusCompleted {
		return j.reject("update_storyboard")
	}
	if sb.IsZero() {
		return fmt.Errorf("%w: vazio", ErrInvalidStoryboard)
	}
	j.storyboard = sb
	j.record(StoryboardUpdated{baseEvent{j.id, now}})
	if j.status == StatusCompleted {
		j.renderID, j.outputKey, j.renderProgress = "", "", 0
		j.transition(StatusStoryboardReady, now, "storyboard editado após render")
		return nil
	}
	j.updatedAt = now
	return nil
}

// RequestRender: STORYBOARD_READY|COMPLETED → RENDER_QUEUED.
// Idempotente: repetir com o mesmo renderID enquanto o render está na fila,
// em andamento ou concluído não altera nada.
func (j *VideoJob) RequestRender(renderID RenderID, now time.Time) (changed bool, err error) {
	if renderID == "" {
		return false, fmt.Errorf("%w: render id vazio", ErrInvalidID)
	}
	if renderID == j.renderID {
		switch j.status {
		case StatusRenderQueued, StatusRendering, StatusCompleted:
			return false, nil
		}
	}
	if j.status != StatusStoryboardReady && j.status != StatusCompleted {
		return false, j.reject("request_render")
	}
	j.renderID = renderID
	j.renderProgress = 0
	j.outputKey = ""
	j.transition(StatusRenderQueued, now, "")
	return true, nil
}

// StartRendering: RENDER_QUEUED → RENDERING. O renderID protege contra worker com tarefa obsoleta.
func (j *VideoJob) StartRendering(renderID RenderID, now time.Time) error {
	if err := j.checkRender(renderID); err != nil {
		return err
	}
	if j.status == StatusRendering {
		return nil // reentrega da fila: idempotente
	}
	if j.status != StatusRenderQueued {
		return j.reject("start_rendering")
	}
	j.transition(StatusRendering, now, "")
	return nil
}

// ReportRenderProgress atualiza o progresso (0..1), apenas em RENDERING e de forma monotônica.
// Retorna changed=false quando o valor não avança (eventos duplicados/fora de ordem).
func (j *VideoJob) ReportRenderProgress(renderID RenderID, progress float64, now time.Time) (changed bool, err error) {
	if err := j.checkRender(renderID); err != nil {
		return false, err
	}
	if j.status != StatusRendering {
		return false, j.reject("report_render_progress")
	}
	if progress < 0 || progress > 1 || progress != progress { // NaN
		return false, fmt.Errorf("%w: %v", ErrInvalidProgress, progress)
	}
	if progress <= j.renderProgress {
		return false, nil
	}
	j.renderProgress = progress
	j.updatedAt = now
	j.record(RenderProgressed{baseEvent: baseEvent{j.id, now}, RenderID: renderID, Progress: progress})
	return true, nil
}

// CompleteRender: RENDERING → COMPLETED. Idempotente para o mesmo renderID/outputKey.
func (j *VideoJob) CompleteRender(renderID RenderID, outputKey string, now time.Time) error {
	if err := j.checkRender(renderID); err != nil {
		return err
	}
	if outputKey == "" {
		return fmt.Errorf("%w: output key vazio", ErrInvalidID)
	}
	if j.status == StatusCompleted && j.outputKey == outputKey {
		return nil
	}
	if j.status != StatusRendering {
		return j.reject("complete_render")
	}
	j.outputKey = outputKey
	j.renderProgress = 1
	j.transition(StatusCompleted, now, "")
	return nil
}

// Fail move um job ativo para FAILED, guardando o estado de origem para retry.
func (j *VideoJob) Fail(reason string, now time.Time) error {
	if !j.status.isFailable() {
		return j.reject("fail")
	}
	if reason == "" {
		reason = "erro desconhecido"
	}
	j.failure = &Failure{Reason: reason, From: j.status, At: now}
	j.transition(StatusFailed, now, reason)
	return nil
}

// Retry retoma um job em FAILED na etapa de fila correspondente:
// falhas de roteiro voltam para GENERATING_STORYBOARD; falhas de render para RENDER_QUEUED
// (mesmo renderID — o worker descarta parciais e o arquivo final é idempotente).
func (j *VideoJob) Retry(now time.Time) error {
	if j.status != StatusFailed || j.failure == nil {
		return j.reject("retry")
	}
	if j.retryCount >= MaxRetries {
		return fmt.Errorf("%w: %d tentativas", ErrRetryLimit, j.retryCount)
	}
	var target JobStatus
	switch j.failure.From {
	case StatusDraft, StatusGeneratingStoryboard:
		target = StatusGeneratingStoryboard
	case StatusRenderQueued, StatusRendering:
		target = StatusRenderQueued
		j.renderProgress = 0
	default:
		return j.reject("retry")
	}
	j.retryCount++
	j.failure = nil
	j.transition(target, now, "retry")
	return nil
}

// PullEvents devolve os eventos pendentes e limpa a lista (chamar após persistir).
func (j *VideoJob) PullEvents() []Event {
	events := j.events
	j.events = nil
	return events
}

// --- Leitura ----------------------------------------------------------------

func (j *VideoJob) ID() JobID               { return j.id }
func (j *VideoJob) Prompt() Prompt          { return j.prompt }
func (j *VideoJob) Template() TemplateRef   { return j.template }
func (j *VideoJob) Status() JobStatus       { return j.status }
func (j *VideoJob) Storyboard() Storyboard  { return j.storyboard }
func (j *VideoJob) RenderID() RenderID      { return j.renderID }
func (j *VideoJob) RenderProgress() float64 { return j.renderProgress }
func (j *VideoJob) OutputKey() string       { return j.outputKey }
func (j *VideoJob) RetryCount() int         { return j.retryCount }
func (j *VideoJob) Version() int            { return j.version }
func (j *VideoJob) CreatedAt() time.Time    { return j.createdAt }
func (j *VideoJob) UpdatedAt() time.Time    { return j.updatedAt }

// Failure retorna uma cópia do registro de falha (nil se não falhou).
func (j *VideoJob) Failure() *Failure {
	if j.failure == nil {
		return nil
	}
	f := *j.failure
	return &f
}

// --- Internos ---------------------------------------------------------------

func (j *VideoJob) transition(to JobStatus, now time.Time, reason string) {
	from := j.status
	j.status = to
	j.updatedAt = now
	j.record(StatusChanged{baseEvent: baseEvent{j.id, now}, From: from, To: to, Reason: reason})
}

func (j *VideoJob) record(e Event) { j.events = append(j.events, e) }

func (j *VideoJob) reject(action string) error {
	return &TransitionError{From: j.status, Action: action}
}

func (j *VideoJob) checkRender(renderID RenderID) error {
	if renderID == "" || renderID != j.renderID {
		return fmt.Errorf("%w: recebido %q, atual %q", ErrRenderMismatch, renderID, j.renderID)
	}
	return nil
}
