package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

const storyboardTool = "submit_storyboard"

// AnthropicConfig configura o adapter da Messages API.
type AnthropicConfig struct {
	APIKey    string
	Model     string // ex.: claude-opus-5-5
	BaseURL   string // padrão https://api.anthropic.com
	MaxTokens int
}

// AnthropicGenerator usa uma ferramenta cujo input_schema é o JSON Schema do template:
// a resposta estruturada chega como bloco tool_use. tool_choice fica em "auto"
// (compatível com extended thinking) e o system prompt instrui a chamar a ferramenta;
// se o modelo responder em texto, o JSON é extraído do texto.
type AnthropicGenerator struct {
	cfg    AnthropicConfig
	client *http.Client
}

var _ ports.StoryboardGenerator = (*AnthropicGenerator)(nil)

func NewAnthropicGenerator(cfg AnthropicConfig, client *http.Client) *AnthropicGenerator {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 8192
	}
	return &AnthropicGenerator{cfg: cfg, client: client}
}

type anthropicRequest struct {
	Model      string           `json:"model"`
	MaxTokens  int              `json:"max_tokens"`
	System     []anthropicBlock `json:"system"`
	Messages   []anthropicMsg   `json:"messages"`
	Tools      []anthropicTool  `json:"tools"`
	ToolChoice map[string]any   `json:"tool_choice"`
}

type anthropicBlock struct {
	Type         string         `json:"type"`
	Text         string         `json:"text"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicTool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"input_schema"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

type anthropicResponse struct {
	Model   string `json:"model"`
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func (g *AnthropicGenerator) Generate(ctx context.Context, in ports.GenerateInput) (ports.GenerateOutput, error) {
	schema, err := providerSchema(in.JSONSchema)
	if err != nil {
		return ports.GenerateOutput{}, err
	}
	ephemeral := map[string]any{"type": "ephemeral"}
	body := anthropicRequest{
		Model:     g.cfg.Model,
		MaxTokens: g.cfg.MaxTokens,
		// System prompt e tool são fixos por template: marcados para prompt caching.
		System: []anthropicBlock{{
			Type:         "text",
			Text:         systemPrompt(in) + "\n\nEntregue o roteiro chamando a ferramenta " + storyboardTool + ".",
			CacheControl: ephemeral,
		}},
		Messages: []anthropicMsg{{Role: "user", Content: userPrompt(in)}},
		Tools: []anthropicTool{{
			Name:         storyboardTool,
			Description:  "Entrega o storyboard final do vídeo, validado pelo JSON Schema do template.",
			InputSchema:  schema,
			CacheControl: ephemeral,
		}},
		ToolChoice: map[string]any{"type": "auto"},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ports.GenerateOutput{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.BaseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return ports.GenerateOutput{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", g.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := g.client.Do(req)
	if err != nil {
		return ports.GenerateOutput{}, fmt.Errorf("anthropic: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return ports.GenerateOutput{}, fmt.Errorf("anthropic: lendo resposta: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ports.GenerateOutput{}, &ProviderError{Provider: "anthropic", StatusCode: resp.StatusCode, Body: truncate(string(raw), 500)}
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ports.GenerateOutput{}, fmt.Errorf("anthropic: resposta inválida: %w", err)
	}
	out := ports.GenerateOutput{
		Model:        nonEmpty(parsed.Model, g.cfg.Model),
		InputTokens:  parsed.Usage.InputTokens + parsed.Usage.CacheCreationInputTokens + parsed.Usage.CacheReadInputTokens,
		OutputTokens: parsed.Usage.OutputTokens,
	}

	for _, block := range parsed.Content {
		if block.Type == "tool_use" && block.Name == storyboardTool {
			sb, err := extractJSON(string(block.Input))
			if err != nil {
				return out, fmt.Errorf("anthropic: %w", err)
			}
			out.Storyboard = sb
			return out, nil
		}
	}
	for _, block := range parsed.Content {
		if block.Type == "text" {
			if sb, err := extractJSON(block.Text); err == nil {
				out.Storyboard = sb
				return out, nil
			}
		}
	}
	return out, fmt.Errorf("anthropic (stop_reason=%s): %w", parsed.StopReason, ErrNoStructuredOutput)
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
