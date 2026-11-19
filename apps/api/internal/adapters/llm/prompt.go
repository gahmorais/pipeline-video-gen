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
	LogoAsset       = "assets/logo.png"
	ProductAsset    = "assets/product-placeholder.png"
	ClipAsset       = "assets/app-promo/sample-clip.mp4"
	SoundtrackAsset = "assets/app-promo/sample-track.mp3"
)

// templateGuide são as instruções específicas de cada template: papel, assets permitidos
// e estrutura recomendada. Fixas por template → elegíveis a prompt caching.
type templateGuide struct {
	role  string
	rules []string
}

var templateGuides = map[string]templateGuide{
	"promo": {
		role: "roteirista de vídeos promocionais curtos para varejo (formato horizontal)",
		rules: []string{
			fmt.Sprintf(`Use apenas estes assets: logo %q; imagem de produto %q. Nunca invente URLs ou caminhos.`, LogoAsset, ProductAsset),
			`"brand.primary" é uma cor hexadecimal #RRGGBB coerente com o pedido (padrão #0B5FFF).`,
			`Estrutura recomendada: uma cena "title" de abertura, de 1 a 6 cenas "product" e uma cena "cta" final.`,
			`O vídeo total deve ficar entre 6 e 45 segundos, salvo pedido explícito.`,
			`Preços no formato brasileiro (ex.: "R$ 499,90") apenas quando informados ou claramente implícitos no pedido; caso contrário use "Consulte".`,
		},
	},
	"app-promo": {
		role: "roteirista de vídeos verticais (Reels, TikTok, Shorts) para divulgar apps e jogos mobile",
		rules: []string{
			fmt.Sprintf(`Use apenas estes assets: ícone do app %q; trecho de gameplay %q; screenshot %q; trilha %q. Nunca invente URLs ou caminhos — o usuário troca os assets depois.`,
				LogoAsset, ClipAsset, ProductAsset, SoundtrackAsset),
			`"brand.appName" é o nome do app citado no pedido; "brand.primary" é uma cor hexadecimal #RRGGBB coerente com a identidade do app.`,
			`Estrutura recomendada: gancho forte nos 3 primeiros segundos ("title" curto ou "clip"), alternância entre cenas "clip" (gameplay) e "feature" (recursos), e uma cena "cta" final com as lojas citadas no pedido (padrão: "google-play").`,
			`Prefira cenas "clip" para mostrar o app em uso; use "caption" curta para explicar o que aparece.`,
			`Inclua "soundtrack" com volume entre 0.4 e 0.7.`,
			`Use a duração total pedida; sem pedido explícito, entre 15 e 30 segundos.`,
		},
	},
}

// systemPrompt é fixo por template — elegível a prompt caching nos provedores.
func systemPrompt(in ports.GenerateInput) string {
	guide, ok := templateGuides[in.Template.ID]
	if !ok {
		guide = templateGuide{role: "roteirista de vídeos curtos"}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Você é um %s.\n", guide.role)
	fmt.Fprintf(&b, "Sua única tarefa é produzir o roteiro (storyboard) em JSON, conforme o JSON Schema do template %q versão %d.\n\n", in.Template.ID, in.Template.Version)
	b.WriteString("Regras obrigatórias:\n")
	common := []string{
		"Responda somente com dados estruturados que satisfaçam o schema. Nada de código, HTML, Markdown ou comentários.",
		fmt.Sprintf(`"schemaVersion" deve ser %d.`, in.Template.Version),
		"Textos curtos e diretos em português do Brasil, respeitando os limites de tamanho do schema.",
		"Durações (durationSec) dentro dos limites do schema; a soma das durações é a duração do vídeo.",
	}
	for _, r := range append(common, guide.rules...) {
		b.WriteString("- " + r + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
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
