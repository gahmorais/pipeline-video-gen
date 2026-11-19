package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// --- Fallback -------------------------------------------------------------------

// FallbackGenerator tenta o primário (ex.: Gemini Flash); se ele falhar ou devolver
// JSON fora do schema, tenta o secundário (ex.: Claude Opus).
type FallbackGenerator struct {
	primary, secondary ports.StoryboardGenerator
	catalog            ports.TemplateCatalog
	logger             *slog.Logger
}

var _ ports.StoryboardGenerator = (*FallbackGenerator)(nil)

func NewFallbackGenerator(primary, secondary ports.StoryboardGenerator, catalog ports.TemplateCatalog, logger *slog.Logger) *FallbackGenerator {
	return &FallbackGenerator{primary: primary, secondary: secondary, catalog: catalog, logger: logger}
}

func (f *FallbackGenerator) Generate(ctx context.Context, in ports.GenerateInput) (ports.GenerateOutput, error) {
	out, err := f.primary.Generate(ctx, in)
	if err == nil {
		issues, verr := f.catalog.Validate(in.Template, out.Storyboard)
		if verr == nil && len(issues) == 0 {
			return out, nil
		}
		err = fmt.Errorf("saída fora do schema (%d problemas)", len(issues))
		if verr != nil {
			err = verr
		}
	}
	if ctx.Err() != nil {
		return ports.GenerateOutput{}, ctx.Err()
	}
	f.logger.WarnContext(ctx, "llm primário falhou, usando fallback", "job_id", in.JobID, "err", err)

	out2, err2 := f.secondary.Generate(ctx, in)
	if err2 != nil {
		return ports.GenerateOutput{}, errors.Join(fmt.Errorf("primário: %w", err), fmt.Errorf("fallback: %w", err2))
	}
	return out2, nil
}

// --- Métricas -------------------------------------------------------------------

// Price é o preço em USD por milhão de tokens.
type Price struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

// MeteredGenerator registra modelo, tokens, custo estimado e duração de cada chamada.
// Deve envolver cada provedor individualmente (dentro do fallback), para medir as duas tentativas.
type MeteredGenerator struct {
	inner    ports.StoryboardGenerator
	model    string
	price    Price
	recorder ports.LLMUsageRecorder
	logger   *slog.Logger
	now      func() time.Time
}

var _ ports.StoryboardGenerator = (*MeteredGenerator)(nil)

func NewMeteredGenerator(inner ports.StoryboardGenerator, model string, price Price, recorder ports.LLMUsageRecorder, logger *slog.Logger) *MeteredGenerator {
	return &MeteredGenerator{inner: inner, model: model, price: price, recorder: recorder, logger: logger, now: time.Now}
}

func (m *MeteredGenerator) Generate(ctx context.Context, in ports.GenerateInput) (ports.GenerateOutput, error) {
	start := m.now()
	out, err := m.inner.Generate(ctx, in)
	usage := ports.LLMUsage{
		JobID:        in.JobID,
		Model:        nonEmpty(out.Model, m.model),
		InputTokens:  out.InputTokens,
		OutputTokens: out.OutputTokens,
		CostUSD:      EstimateCost(m.price, out.InputTokens, out.OutputTokens),
		Duration:     m.now().Sub(start),
		Success:      err == nil,
	}
	if err != nil {
		usage.Error = truncate(err.Error(), 500)
	}
	m.logger.InfoContext(ctx, "llm usage",
		"job_id", usage.JobID, "model", usage.Model, "input_tokens", usage.InputTokens,
		"output_tokens", usage.OutputTokens, "cost_usd", usage.CostUSD,
		"duration_ms", usage.Duration.Milliseconds(), "success", usage.Success)
	if m.recorder != nil {
		// Métrica nunca derruba a geração.
		if rerr := m.recorder.Record(context.WithoutCancel(ctx), usage); rerr != nil {
			m.logger.WarnContext(ctx, "falha ao registrar uso do llm", "err", rerr)
		}
	}
	return out, err
}

func EstimateCost(p Price, inputTokens, outputTokens int) float64 {
	return (float64(inputTokens)*p.InputPerMTok + float64(outputTokens)*p.OutputPerMTok) / 1_000_000
}

// --- Fixture --------------------------------------------------------------------

// FixtureGenerator devolve um roteiro fixo por template, sem custo. Para desenvolvimento
// local e testes de ponta a ponta sem chave de API (STORYBOARD_PROVIDER=fixture).
type FixtureGenerator struct {
	// ByTemplate mapeia o id do template para o storyboard devolvido.
	ByTemplate map[string][]byte
}

// ErrNoFixture indica template sem fixture configurada.
var ErrNoFixture = errors.New("nenhuma fixture configurada para o template")

func (f FixtureGenerator) Generate(_ context.Context, in ports.GenerateInput) (ports.GenerateOutput, error) {
	sb, ok := f.ByTemplate[in.Template.ID]
	if !ok {
		return ports.GenerateOutput{}, fmt.Errorf("%w %q", ErrNoFixture, in.Template.ID)
	}
	return ports.GenerateOutput{Storyboard: append([]byte(nil), sb...), Model: "fixture"}, nil
}
