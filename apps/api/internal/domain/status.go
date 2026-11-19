// Package domain contém o agregado VideoJob e suas regras. Não depende de
// infraestrutura (Postgres, HTTP, LLM, Remotion, filesystem).
package domain

// JobStatus é o estado do agregado VideoJob.
type JobStatus string

const (
	StatusDraft                JobStatus = "DRAFT"
	StatusGeneratingStoryboard JobStatus = "GENERATING_STORYBOARD"
	StatusStoryboardReady      JobStatus = "STORYBOARD_READY"
	StatusRenderQueued         JobStatus = "RENDER_QUEUED"
	StatusRendering            JobStatus = "RENDERING"
	StatusCompleted            JobStatus = "COMPLETED"
	StatusFailed               JobStatus = "FAILED"
)

// AllStatuses lista todos os estados, útil para validação e testes exaustivos.
var AllStatuses = []JobStatus{
	StatusDraft,
	StatusGeneratingStoryboard,
	StatusStoryboardReady,
	StatusRenderQueued,
	StatusRendering,
	StatusCompleted,
	StatusFailed,
}

// IsValid indica se o valor é um status conhecido (usado ao reidratar do banco).
func (s JobStatus) IsValid() bool {
	for _, v := range AllStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// isFailable indica estados "ativos": há trabalho em andamento ou pendente
// que pode falhar. STORYBOARD_READY e COMPLETED aguardam o usuário; FAILED já é terminal.
func (s JobStatus) isFailable() bool {
	switch s {
	case StatusDraft, StatusGeneratingStoryboard, StatusRenderQueued, StatusRendering:
		return true
	default:
		return false
	}
}
