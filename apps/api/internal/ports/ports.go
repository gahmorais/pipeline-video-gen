// Package ports define as interfaces que a camada de aplicação consome.
// Implementações concretas ficam em internal/adapters.
package ports

import (
	"context"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
)

// VideoJobRepository persiste o agregado. Update usa controle de concorrência
// otimista pela versão do job e retorna domain.ErrConcurrentUpdate em conflito.
type VideoJobRepository interface {
	Get(ctx context.Context, id domain.JobID) (*domain.VideoJob, error)
	Create(ctx context.Context, job *domain.VideoJob) error
	Update(ctx context.Context, job *domain.VideoJob) error
}

// TaskKind identifica o tipo de trabalho assíncrono.
type TaskKind string

const (
	TaskGenerateStoryboard TaskKind = "storyboard"
	TaskRender             TaskKind = "render"
)

// Task é uma unidade de trabalho na fila.
type Task struct {
	Kind     TaskKind
	JobID    domain.JobID
	RenderID domain.RenderID // apenas para TaskRender
}

// TaskQueue é o lado produtor da fila.
type TaskQueue interface {
	Enqueue(ctx context.Context, task Task) error
}

// Repositories agrupa os repositórios que participam de uma mesma transação.
type Repositories struct {
	Jobs  VideoJobRepository
	Queue TaskQueue
}

// UnitOfWork executa fn de forma atômica: job e fila são gravados juntos
// (regra 2 do CLAUDE.md: criar job e enfileirar na mesma transação).
type UnitOfWork interface {
	Do(ctx context.Context, fn func(ctx context.Context, repos Repositories) error) error
}

// GenerateInput é o pedido ao LLM.
type GenerateInput struct {
	JobID    domain.JobID
	Prompt   domain.Prompt
	Template domain.TemplateRef
	// JSONSchema do template, usado como structured output.
	JSONSchema []byte
}

// GenerateOutput é a resposta do LLM com dados de uso para métricas de custo.
type GenerateOutput struct {
	Storyboard   []byte
	Model        string
	InputTokens  int
	OutputTokens int
}

// StoryboardGenerator gera o roteiro a partir do prompt (LLM).
type StoryboardGenerator interface {
	Generate(ctx context.Context, in GenerateInput) (GenerateOutput, error)
}

// ValidationIssue descreve um problema de schema em um caminho do JSON.
type ValidationIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// TemplateCatalog conhece os templates suportados e seus JSON Schemas.
type TemplateCatalog interface {
	// Latest retorna a versão mais recente de um template.
	Latest(templateID string) (domain.TemplateRef, bool)
	// JSONSchema retorna o schema de uma versão específica.
	JSONSchema(ref domain.TemplateRef) ([]byte, bool)
	// Validate valida o storyboard contra o schema do template.
	// Retorna lista vazia quando válido.
	Validate(ref domain.TemplateRef, storyboard []byte) ([]ValidationIssue, error)
}

// EventPublisher entrega eventos de domínio (ex.: SSE). Chamado após o commit;
// falhas não desfazem a operação.
type EventPublisher interface {
	Publish(ctx context.Context, events []domain.Event)
}

// IDGenerator gera identificadores únicos.
type IDGenerator interface {
	NewJobID() domain.JobID
	NewRenderID() domain.RenderID
}

// Clock abstrai o tempo para testes determinísticos.
type Clock interface {
	Now() time.Time
}
