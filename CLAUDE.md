# CLAUDE.md — Pipeline de geração de vídeo (Remotion)

## Visão geral

Sistema para gerar vídeos a partir de um prompt em linguagem natural.

- Um **LLM comercial** (Gemini Flash / Claude Opus) gera **apenas um JSON estruturado** (roteiro/storyboard): cenas, textos, durações, cores, referências a assets.
- O **Remotion** renderiza esse JSON em MP4 usando **composições React fixas, versionadas e escritas por nós**.
- O LLM **nunca** gera código TSX, nunca gera pixels e nunca define frames.
- O usuário visualiza o roteiro instantaneamente no browser via `@remotion/player` (custo zero), edita e só então dispara o render (parte cara).

## Restrições do ambiente (decisões fechadas)

- **Tudo roda localmente** em um **MacBook Air M2, 16 GB RAM** (sem ventoinha → throttling térmico em carga sustentada).
- Sem cloud: sem S3, sem Lambda, sem fila gerenciada.
- **Único custo variável** é a API do LLM (~US$ 0,02 Flash / ~US$ 0,15 Opus por geração de roteiro). Duração do vídeo afeta tempo de render, não custo monetário.
- **Render worker roda nativo no macOS** (não em Docker — a VM Linux do Docker no Mac adiciona overhead e consome RAM).
- Docker só para Postgres (opcional para a API).
- Renderizar com o notebook **na tomada**.

## Arquitetura

Hexagonal (ports & adapters) em todos os serviços. Domínio não conhece Remotion, LLM, Postgres nem filesystem.

```
web (Next.js + Remotion Player)
   │ REST + SSE
api (Go) ──── dono único do estado do agregado VideoJob
   │
Postgres ──── estado + fila (SELECT ... FOR UPDATE SKIP LOCKED + LISTEN/NOTIFY)
   │
   ├── storyboard worker (Go, pode ser goroutine da api) ──▶ LLM API (Gemini / Claude)
   └── render worker (Node/TS, nativo) ──▶ Remotion + Chrome Headless Shell + FFmpeg ──▶ ./media (FS local)
```

### Regras arquiteturais

1. **A API é a única dona do estado do `VideoJob`.** Workers reportam resultado/progresso; a transição de estado é feita pelo domínio (na API ou via use case compartilhado que passa pelo agregado). Nunca atualizar status com SQL solto em handler/worker.
2. **Fila = tabela no Postgres.** Criar job e enfileirar na **mesma transação** (sem outbox necessária neste cenário). Jobs travados (worker morto) detectados por `locked_at` antigo e devolvidos à fila.
3. **Portas obrigatórias** (permitem migrar depois para cloud sem tocar no domínio):
   - `StoryboardGenerator` — LLM
   - `JobQueue` — Postgres hoje, NATS JetStream no futuro
   - `MediaStorage` — filesystem local hoje, S3/MinIO no futuro
   - `Renderer` — Remotion local hoje, Remotion Lambda no futuro
   - `VideoJobRepository`
   - `EventPublisher` / notificação SSE
4. **Schema como fonte única de verdade** (ver seção Contrato).
5. **Render: 1 job por vez** na máquina. Paralelismo é interno ao Remotion (`concurrency`). Dois renders simultâneos não aumentam throughput.

## Stack

| Componente | Tecnologia |
|---|---|
| API | Go (stdlib `net/http` ou chi), SSE para progresso |
| Storyboard worker | Go |
| Render worker | Node.js + TypeScript (strict) + `@remotion/renderer` |
| Composições | React + Remotion (pacote compartilhado) |
| Frontend | Next.js + `@remotion/player` |
| Banco / fila | PostgreSQL |
| Storage | Filesystem local (`./media`), servido pela API com `http.ServeContent` (suporta Range/seek) |
| Validação | Zod 4 (TS) → JSON Schema gerado com `z.toJSONSchema` → `santhosh-tekuri/jsonschema` (Go) |
| LLM | Gemini Flash (padrão) + Claude Opus (`claude-opus-5-5`, fallback/premium) — confirmar IDs atuais dos modelos |

## Estrutura do monorepo

```
apps/
  api/                          # Go
    cmd/api/                    # main: composição das dependências (DI manual)
    cmd/storyboard-worker/      # opcional: pode rodar dentro da api
    internal/
      domain/                   # VideoJob, Storyboard, JobStatus, erros de domínio, eventos
      application/              # use cases: CreateVideo, GenerateStoryboard, UpdateStoryboard, RequestRender, CompleteRender, FailJob
      ports/                    # interfaces
      adapters/
        http/                   # handlers REST + SSE
        postgres/               # repositórios + fila (SKIP LOCKED, LISTEN/NOTIFY)
        llm/                    # gemini.go, anthropic.go, fallback.go, metered.go
        storage/                # localfs.go
  render-worker/                # Node/TS
    src/
      domain/
      application/RenderVideo.ts
      ports/                    # Renderer, MediaStorage, JobQueue, ProgressReporter
      adapters/
        remotion-local/
        postgres-queue/
        localfs/
      main.ts                   # composition root
  web/                          # Next.js
packages/
  video-schema/                 # Zod schemas + script que gera JSON Schema
  compositions/                 # componentes Remotion (usados pelo web e pelo render-worker)
media/                          # MP4 gerados (gitignored)
docker-compose.yml              # postgres
```

## Contrato: schema do storyboard

- Definido **uma vez** em `packages/video-schema` com Zod.
- Usado em `<Composition schema={...}>` (Studio gera formulário e valida props).
- `pnpm --filter @videogen/video-schema generate` gera `promo.v1.schema.json` (via `z.toJSONSchema`, Zod 4 — mesma versão usada pelo Remotion), que é:
  - usado pelo Go para validar a saída do LLM e os PATCHs do usuário;
  - enviado ao LLM como structured output / tool schema.
- Todo job grava `templateId`, `templateVersion` e `schemaVersion`.
- Limites obrigatórios no schema (`max` de cenas, tamanho de textos, `durationSec` mínimo/máximo) para evitar vídeos absurdos.
- Duração total calculada no `calculateMetadata`: `durationInFrames = sum(scene.durationSec) * fps`. O LLM nunca define frames.

Exemplo de referência:

```ts
export const PromoSchema = z.object({
  schemaVersion: z.literal(1),
  brand: z.object({
    primary: z.string().regex(/^#[0-9a-f]{6}$/i),
    logoUrl: z.string(),
  }),
  scenes: z.array(z.discriminatedUnion("type", [
    z.object({ type: z.literal("title"), text: z.string().max(60), durationSec: z.number().min(1).max(8) }),
    z.object({ type: z.literal("product"), name: z.string(), price: z.string(), imageUrl: z.string(), durationSec: z.number().min(1).max(10) }),
    z.object({ type: z.literal("cta"), text: z.string().max(40), durationSec: z.number().min(1).max(8) }),
  ])).min(1).max(12),
});
```

## Máquina de estados do VideoJob

```
DRAFT
  → GENERATING_STORYBOARD
  → STORYBOARD_READY  ⇄ (edição pelo usuário via PATCH, sem chamar LLM)
  → RENDER_QUEUED
  → RENDERING
  → COMPLETED

qualquer estado ativo → FAILED (com reason)
FAILED → retry volta ao estado anterior ao erro
```

- Transições implementadas **no agregado** (`domain`). Transição inválida = erro de domínio.
- Testes de tabela cobrindo todas as transições válidas e inválidas.

## Fluxo ponta a ponta

1. `POST /videos {prompt, templateId}` → cria `VideoJob(DRAFT)` + job de roteiro na fila (mesma transação).
2. Storyboard worker consome, chama `StoryboardGenerator`, valida contra o JSON Schema, persiste → `STORYBOARD_READY` → SSE notifica o frontend.
3. Frontend renderiza `<Player>` com o storyboard como `inputProps`. Usuário edita → `PATCH /videos/{id}/storyboard` (validado).
4. `POST /videos/{id}/render` (com idempotency key) → `RENDER_QUEUED`.
5. Render worker consome, chama `renderMedia()`, reporta progresso (~a cada 5%) → SSE.
6. Saída em `media/{jobId}/{renderId}.mp4` → `COMPLETED` → frontend recebe URL servida pela API.

## LLM

- Adapters: `GeminiStoryboardGenerator`, `AnthropicStoryboardGenerator`.
- Decorators:
  - `FallbackGenerator`: tenta Gemini Flash; se erro ou saída inválida no schema, usa Opus.
  - `MeteredGenerator`: grava `model`, `input_tokens`, `output_tokens`, custo estimado por job.
- Usar **structured output / JSON Schema** do provedor sempre que disponível.
- **Prompt caching** do system prompt + schema (são fixos).
- Ajustes pequenos do usuário editam o JSON localmente; **não** regenerar via LLM.
- Chaves de API via variáveis de ambiente (`GEMINI_API_KEY`, `ANTHROPIC_API_KEY`). Nunca commitar.

## Render worker (Remotion)

- **Bundle no build/startup, nunca por job.** Gerar o bundle de `packages/compositions` uma vez e reutilizar o `serveUrl`.
- `renderMedia()` com:
  - `codec: "h264"`, 1080p, 30 fps (evitar 60 fps e 4K nesta máquina);
  - `concurrency: 4` como ponto de partida — calibrar com `npx remotion benchmark`;
  - `onProgress` → reportar progresso;
  - timeouts definidos (`timeoutInMilliseconds` e timeout global do job).
- Aceleração de hardware (`hardwareAcceleration: "if-possible"`) no macOS só se aplica a ProRes na versão documentada — não depender disso para H.264.
- **Idempotência:** `renderId` determina o caminho do arquivo; se já existir e estiver íntegro, apenas reportar conclusão.
- Retries com backoff; após N tentativas → `FAILED`.
- Template versionado: re-render de job antigo deve gerar o mesmo vídeo.

## Regras para escrever composições Remotion

- Tudo derivado de `useCurrentFrame()` / `useVideoConfig()`. Cada frame é função pura de `(frame, props)`.
- **Proibido:** `Math.random()`, `Date.now()`, `setTimeout`, `requestAnimationFrame`, animações/transições CSS baseadas em tempo.
- Aleatoriedade: `random(seed)` do Remotion.
- Animações com `interpolate()` (com `extrapolateRight: "clamp"`) e `spring()`.
- Cenas como componentes isolados dentro de `<Series>` / `<Sequence>` (tempo local começa em 0).
- Vídeos embutidos: `<OffthreadVideo>`. Imagens: `<Img>`. Assets locais: `staticFile()`.
- Recursos assíncronos (fontes, fetch): `delayRender()` / `continueRender()`.
- Evitar efeitos caros por frame: `filter: blur()`, `box-shadow` grandes, muitos vídeos simultâneos.
- Trabalho pesado fora do corpo do componente (`useMemo` ou pré-processado nas props).
- Instalar as skills oficiais: `npx remotion skills add`.

## Segurança

- O LLM gera dados, nunca código. Nada vindo do LLM é executado.
- Validação do JSON **na API e no worker** (defesa em profundidade).
- Sanitizar textos do LLM antes de exibir no frontend (XSS).
- Assets do usuário: upload via API com limite de tamanho e whitelist de MIME types. O LLM só referencia assets já existentes/validados — nunca URLs arbitrárias.
- Rate limit em `POST /render`.

## Observabilidade

- `job_id` propagado como correlation/trace id em todos os logs (API → fila → workers).
- Logs estruturados (Go: `log/slog`; Node: pino).
- Métricas: tempo em fila, segundos de render por segundo de vídeo, taxa de falha por template, tokens/custo do LLM por job.
- ETA na UI calculado a partir do histórico real de render.

## Operação local

- Postgres via `docker compose up -d postgres`.
- API, workers e web rodando nativos.
- Job de limpeza: remover renders antigos de `./media` após X dias (configurável).

## Convenções de código

### Go
- `internal/` para tudo que não é `cmd/`.
- Interfaces definidas no consumidor (`ports/`), pequenas e focadas.
- `context.Context` como primeiro parâmetro em toda operação de I/O.
- Erros com wrap (`fmt.Errorf("...: %w", err)`) e erros de domínio sentinela/tipados.
- DI manual no `main` (composition root). Sem frameworks de DI.
- Testes table-driven; domínio e use cases testados sem infraestrutura (fakes das portas).
- Testes de integração do Postgres com testcontainers.

### TypeScript
- `strict: true`, sem `any`.
- Mesma separação domain / application / ports / adapters no render worker.
- Composições testadas visualmente no Studio e no Player com fixtures de storyboard.

## Roadmap

1. `packages/video-schema` + uma composição simples (`Promo`) + Studio funcionando.
2. Render worker CLI renderizando um JSON fixo localmente (calibrar concurrency).
3. API Go: domínio, máquina de estados, Postgres, fila, SSE.
4. Storyboard worker com Gemini Flash + validação + fallback Opus + métrica de tokens.
5. Frontend: prompt → preview no Player → edição → render → download.
6. Observabilidade, limpeza de mídia, quotas.
7. (Futuro) Migração seletiva para cloud trocando adapters: fila → NATS, FS → S3, render → Remotion Lambda.

## Observações

- **Licença Remotion:** não é MIT. Gratuita para indivíduos e empresas pequenas; empresas maiores precisam de licença comercial. Verificar remotion.dev/license antes de uso corporativo.
- Ao propor mudanças, priorizar clareza arquitetural, baixo acoplamento e as melhores práticas idiomáticas de Go e TypeScript.
