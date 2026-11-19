// Package llm implementa ports.StoryboardGenerator para provedores de LLM
// (Gemini, Anthropic) e decorators (fallback, métricas).
package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// Assets que o LLM pode referenciar. O LLM nunca inventa URLs (regra de segurança).
const (
	LogoAsset    = "assets/logo.png"
	ProductAsset = "assets/product-placeholder.png"
)

// systemPrompt é fixo por template — elegível a prompt caching nos provedores.
func systemPrompt(in ports.GenerateInput) string {
	return fmt.Sprintf(`Você é um roteirista de vídeos promocionais curtos para varejo.
Sua única tarefa é produzir o roteiro (storyboard) em JSON, conforme o JSON Schema do template %q versão %d.

Regras obrigatórias:
- Responda somente com dados estruturados que satisfaçam o schema. Nada de código, HTML, Markdown ou comentários.
- "schemaVersion" deve ser %d.
- Use apenas estes assets: logo "%s"; imagem de produto "%s". Nunca invente URLs ou caminhos.
- "brand.primary" é uma cor hexadecimal #RRGGBB coerente com o pedido (padrão #0B5FFF).
- Estrutura recomendada: uma cena "title" de abertura, de 1 a 6 cenas "product" e uma cena "cta" final.
- Textos curtos e diretos em português do Brasil, respeitando os limites de tamanho do schema.
- Durações (durationSec) entre os limites do schema; o vídeo total deve ficar entre 6 e 45 segundos, salvo pedido explícito.
- Preços no formato brasileiro (ex.: "R$ 499,90") apenas quando informados ou claramente implícitos no pedido; caso contrário use "Consulte".`,
		in.Template.ID, in.Template.Version, in.Template.Version, LogoAsset, ProductAsset)
}

func userPrompt(in ports.GenerateInput) string {
	return "Pedido do usuário:\n" + string(in.Prompt)
}

// providerSchema remove metadados que alguns provedores rejeitam ($schema, $id).
func providerSchema(raw []byte) (map[string]any, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("schema inválido: %w", err)
	}
	delete(schema, "$schema")
	delete(schema, "$id")
	return schema, nil
}

// extractJSON aceita a resposta crua ou envolta em cerca de Markdown (```json ... ```),
// garantindo que o resultado seja um único objeto JSON.
func extractJSON(text string) ([]byte, error) {
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "```") {
		t = strings.TrimPrefix(t, "```json")
		t = strings.TrimPrefix(t, "```")
		t = strings.TrimSuffix(strings.TrimSpace(t), "```")
		t = strings.TrimSpace(t)
	}
	if !strings.HasPrefix(t, "{") || !json.Valid([]byte(t)) {
		return nil, ErrNoStructuredOutput
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(t)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ErrNoStructuredOutput indica que o provedor respondeu sem o JSON esperado.
var ErrNoStructuredOutput = errors.New("resposta do LLM sem JSON estruturado")

// ProviderError carrega status HTTP e trecho do corpo (sem credenciais) para diagnóstico.
type ProviderError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.Provider, e.StatusCode, e.Body)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
