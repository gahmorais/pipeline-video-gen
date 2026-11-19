package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// GeminiConfig configura o adapter da Gemini API (generateContent).
type GeminiConfig struct {
	APIKey  string
	Model   string // ex.: gemini-3.5-flash — confirmar o id vigente
	BaseURL string // padrão https://generativelanguage.googleapis.com
}

// GeminiGenerator usa structured output: responseMimeType=application/json +
// responseJsonSchema com o JSON Schema do template. O cache de prompt do
// Gemini é implícito para prefixos repetidos (system instruction fixa).
type GeminiGenerator struct {
	cfg    GeminiConfig
	client *http.Client
}

var _ ports.StoryboardGenerator = (*GeminiGenerator)(nil)

func NewGeminiGenerator(cfg GeminiConfig, client *http.Client) *GeminiGenerator {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com"
	}
	return &GeminiGenerator{cfg: cfg, client: client}
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	SystemInstruction geminiContent   `json:"systemInstruction"`
	Contents          []geminiContent `json:"contents"`
	GenerationConfig  map[string]any  `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	ModelVersion string `json:"modelVersion"`
}

func (g *GeminiGenerator) Generate(ctx context.Context, in ports.GenerateInput) (ports.GenerateOutput, error) {
	schema, err := providerSchema(in.JSONSchema)
	if err != nil {
		return ports.GenerateOutput{}, err
	}
	body := geminiRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: systemPrompt(in)}}},
		Contents:          []geminiContent{{Role: "user", Parts: []geminiPart{{Text: userPrompt(in)}}}},
		GenerationConfig: map[string]any{
			"responseMimeType":   "application/json",
			"responseJsonSchema": schema,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ports.GenerateOutput{}, err
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", g.cfg.BaseURL, url.PathEscape(g.cfg.Model))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return ports.GenerateOutput{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Chave no header, nunca na URL (URLs vão para logs).
	req.Header.Set("x-goog-api-key", g.cfg.APIKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return ports.GenerateOutput{}, fmt.Errorf("gemini: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return ports.GenerateOutput{}, fmt.Errorf("gemini: lendo resposta: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ports.GenerateOutput{}, &ProviderError{Provider: "gemini", StatusCode: resp.StatusCode, Body: truncate(string(raw), 500)}
	}

	var parsed geminiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ports.GenerateOutput{}, fmt.Errorf("gemini: resposta inválida: %w", err)
	}
	out := ports.GenerateOutput{
		Model:       nonEmpty(parsed.ModelVersion, g.cfg.Model),
		InputTokens: parsed.UsageMetadata.PromptTokenCount,
		// Tokens de "thinking" são cobrados como saída.
		OutputTokens: parsed.UsageMetadata.CandidatesTokenCount + parsed.UsageMetadata.ThoughtsTokenCount,
	}
	if len(parsed.Candidates) == 0 {
		return out, fmt.Errorf("gemini: sem candidatos: %w", ErrNoStructuredOutput)
	}
	var text bytes.Buffer
	for _, p := range parsed.Candidates[0].Content.Parts {
		text.WriteString(p.Text)
	}
	sb, err := extractJSON(text.String())
	if err != nil {
		return out, fmt.Errorf("gemini (finishReason=%s): %w", parsed.Candidates[0].FinishReason, err)
	}
	out.Storyboard = sb
	return out, nil
}
