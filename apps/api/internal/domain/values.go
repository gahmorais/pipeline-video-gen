package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxPromptChars limita o prompt enviado ao LLM (custo e abuso).
const MaxPromptChars = 2000

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// JobID identifica um VideoJob. Mesmo formato aceito pelo render worker
// (seguro para compor caminhos de arquivo).
type JobID string

// RenderID identifica uma tentativa de render (determina o arquivo de saída).
type RenderID string

func ParseJobID(s string) (JobID, error) {
	if !idPattern.MatchString(s) {
		return "", fmt.Errorf("%w: job id %q", ErrInvalidID, s)
	}
	return JobID(s), nil
}

func ParseRenderID(s string) (RenderID, error) {
	if !idPattern.MatchString(s) {
		return "", fmt.Errorf("%w: render id %q", ErrInvalidID, s)
	}
	return RenderID(s), nil
}

// Prompt é o pedido em linguagem natural do usuário.
type Prompt string

func NewPrompt(s string) (Prompt, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", fmt.Errorf("%w: vazio", ErrInvalidPrompt)
	}
	if n := utf8.RuneCountInString(trimmed); n > MaxPromptChars {
		return "", fmt.Errorf("%w: %d caracteres (máximo %d)", ErrInvalidPrompt, n, MaxPromptChars)
	}
	return Prompt(trimmed), nil
}

// TemplateRef aponta para um template versionado (gravado no job para re-render reprodutível).
type TemplateRef struct {
	ID      string
	Version int
}

func NewTemplateRef(id string, version int) (TemplateRef, error) {
	if !idPattern.MatchString(id) || version <= 0 {
		return TemplateRef{}, fmt.Errorf("%w: %q@%d", ErrInvalidTemplate, id, version)
	}
	return TemplateRef{ID: id, Version: version}, nil
}

func (t TemplateRef) String() string { return fmt.Sprintf("%s@%d", t.ID, t.Version) }

// Storyboard é o JSON do roteiro. O domínio garante apenas que é um objeto JSON;
// a conformidade com o schema do template é verificada na camada de aplicação
// (porta StoryboardValidator), pois depende de artefato externo (JSON Schema).
type Storyboard struct {
	raw json.RawMessage
}

func NewStoryboard(raw []byte) (Storyboard, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return Storyboard{}, fmt.Errorf("%w: precisa ser um objeto JSON", ErrInvalidStoryboard)
	}
	cp := make([]byte, len(trimmed))
	copy(cp, trimmed)
	return Storyboard{raw: cp}, nil
}

// JSON retorna uma cópia do conteúdo (o agregado não expõe estado mutável).
func (s Storyboard) JSON() json.RawMessage {
	if s.raw == nil {
		return nil
	}
	cp := make([]byte, len(s.raw))
	copy(cp, s.raw)
	return cp
}

func (s Storyboard) IsZero() bool { return len(s.raw) == 0 }
