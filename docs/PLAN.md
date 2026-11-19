# Plano de implementação

Cada etapa é pequena, entregável e **só é commitada após testes passando e validação manual/automática**.
Convenção de commit: `feat(etapa-N): <descrição>` (ou `chore`/`test`/`docs` quando aplicável).

Legenda: ✅ concluída · 🔄 em andamento · ⏳ pendente

| # | Etapa | Entregável | Validação | Status |
|---|---|---|---|---|
| 0 | Bootstrap do monorepo | `CLAUDE.md`, plano, `.gitignore`, pnpm workspace, `docker-compose.yml` (Postgres) | `pnpm install` ok; `docker compose config` ok | ✅ |
| 1 | `packages/video-schema` | Schema Zod `Promo` v1, tipos, fixtures, script que gera `storyboard.schema.json`, helper `totalDurationSec` | Vitest: fixtures válidas passam, inválidas falham (limites); JSON Schema gerado e determinístico | ✅ |
| 2 | `packages/compositions` | Composição Remotion `Promo` (cenas title/product/cta em `<Series>`), `calculateMetadata` a partir de `durationSec`, Root para o Studio | `tsc --noEmit`; Vitest de cálculo de frames; `renderStill` de um frame de cada cena | ✅ |
| 3 | `apps/render-worker` (CLI) | Hexagonal: ports (`Renderer`, `MediaStorage`), use case `RenderVideo`, adapters `remotion-local` (bundle único) e `localfs`; CLI `render <fixture.json>` | Vitest do use case com fakes (validação, idempotência, progresso); render real de fixture curta gerando MP4 válido (ffprobe) | ✅ |
| 4 | `apps/api` — domínio | Agregado `VideoJob`, `JobStatus`, máquina de estados, erros de domínio, eventos | `go test ./internal/domain/...` table-driven com todas as transições válidas/inválidas | ✅ |
| 5 | `apps/api` — portas e use cases | `ports/` + `application/` (CreateVideo, GenerateStoryboard, UpdateStoryboard, RequestRender, ReportRenderProgress, CompleteRender, FailJob, RetryJob) | `go test` com fakes das portas | ✅ |
| 6 | Validação de schema em Go | Adapter que valida storyboard contra `storyboard.schema.json` (embutido) | `go test` com as mesmas fixtures do pacote TS | ✅ |
| 7 | Postgres: repositório + fila | Migrations, `VideoJobRepository`, `JobQueue` (SKIP LOCKED, reclaim de jobs travados, LISTEN/NOTIFY) | Testes de integração contra Postgres real | ⏳ |
| 8 | HTTP + SSE | Handlers REST, SSE de eventos por job, endpoints internos para workers, servir MP4 (`http.ServeContent`) | `httptest` + teste e2e da API com Postgres | ⏳ |
| 9 | LLM adapters | `GeminiStoryboardGenerator`, `AnthropicStoryboardGenerator`, `FallbackGenerator`, `MeteredGenerator`; storyboard worker | `go test` com servidores HTTP fake simulando os provedores | ⏳ |
| 10 | Render worker conectado | Adapter de fila Postgres + reporter HTTP para a API; loop de consumo | Teste e2e: job criado via API → render → `COMPLETED` | ⏳ |
| 11 | Frontend | Next.js: prompt → preview no `Player` → edição → render → progresso SSE → download | Build + teste manual | ⏳ |
| 12 | Operação | Logs estruturados com `job_id`, métricas, limpeza de `./media`, rate limit/quotas | Testes unitários + checklist | ⏳ |

## Decisões de design registradas durante a implementação

- **Workers não escrevem no estado do `VideoJob`.** Eles consomem a fila (tabela de fila no Postgres) e reportam progresso/resultado para a API por endpoints internos. A API aplica as transições no agregado (regra 1 do `CLAUDE.md`).
- **Zod 4 + `z.toJSONSchema` em vez de `zod-to-json-schema`.** O Remotion 4.0.545 depende de Zod 4, que gera JSON Schema draft 2020-12 nativamente. Uma única versão de Zod no monorepo.
- **JSON Schema gerado é commitado em dois lugares**: `packages/video-schema/schemas/` (canônico) e `apps/api/internal/adapters/schema/schemas/` (para `go:embed`, que não acessa arquivos fora do módulo Go). Um teste falha se o arquivo commitado divergir do Zod.
- **Schemas `.strict()`**: propriedades extras são rejeitadas (o LLM não pode injetar campos como `durationInFrames`).
- **Frames por cena arredondados individualmente**: `totalDurationInFrames` = soma dos frames de cada cena, garantindo que o total bate com as `<Series.Sequence>`.
- **Imports relativos sem extensão** (`moduleResolution: Bundler`): o webpack do Remotion não resolve `./x.js` → `x.tsx`.
- **`BROWSER_EXECUTABLE`** (opcional): caminho de um Chrome local quando o download do Chrome Headless Shell não for possível; nesse caso usa-se `chromeMode: "chrome-for-testing"`.
- **`resolveAssetRef`** rejeita esquemas não-http (`javascript:`, `data:`, `file:`) e path traversal; referência inválida vira placeholder em vez de quebrar o render.
- **`MediaStorage` com escrita em duas fases** (`createWriteTarget` → `commit`/`abort`): o renderer escreve num `.partial-<uuid>.mp4` e o commit faz rename atômico. `exists()` nunca vê arquivo incompleto — base da idempotência por `renderId`.
- **Erros com `retryable`**: `InvalidRenderRequestError` nunca é repetido; `RenderFailedError`/`RenderTimeoutError` entram no backoff exponencial (`withRetry`).
- **Progresso em degraus de 5%**, monotônico; falha ao reportar progresso não derruba o render.
- **Medição neste ambiente (Linux, 2 vCPU, concurrency 2)**: fixture `promo-basic` (6,5 s, 195 frames) renderizou em ~21 s. No M2 calibrar com `npx remotion benchmark`.
- **Refinamentos da máquina de estados** (implementados no agregado, cobertos por matriz exaustiva ação × estado):
  - `UpdateStoryboard` também é permitido em `COMPLETED` (volta para `STORYBOARD_READY` e descarta o render anterior) para permitir re-render após edição.
  - `RequestRender`, `StartRendering` e `CompleteRender` são **idempotentes** para o mesmo `renderId` (reentrega da fila / idempotency key).
  - Comandos de render exigem o `renderId` atual (`ErrRenderMismatch`): protege contra worker com tarefa obsoleta.
  - `Retry` volta à **etapa de fila** correspondente (falha de roteiro → `GENERATING_STORYBOARD`; falha de render → `RENDER_QUEUED`), limitado a `MaxRetries = 3`.
  - Estados "ativos" que podem falhar: `DRAFT`, `GENERATING_STORYBOARD`, `RENDER_QUEUED`, `RENDERING`.
- **`Snapshot`/`Rehydrate`** para persistência sem setters públicos; `version` para controle de concorrência otimista no repositório.
- **`UnitOfWork`** (porta) executa job + fila atomicamente; os use cases nunca veem transações SQL. Eventos de domínio são coletados dentro da transação e publicados **somente após o commit**.
- **LLM chamado fora de transação**: `GenerateStoryboard` marca `GENERATING_STORYBOARD` numa transação curta, chama o LLM, e anexa o resultado em outra transação — ignorando o resultado se o job mudou nesse intervalo.
- **Falha do LLM ou saída fora do schema vira `FAILED`** com motivo (não erro de infraestrutura); o usuário pode usar `Retry`.
- **Idempotency key do `POST /render` vira o `renderId`**; comandos idempotentes não incrementam a versão do job.
- **`santhosh-tekuri/jsonschema/v5`** (em vez da v6): sem dependências transitivas (a v6 puxa `golang.org/x/text`). Suporta draft 2020-12. Schemas embutidos via `go:embed`, compilados no startup (fail fast) e sem resolução de `$ref` remoto.
- **Paridade TS ↔ Go**: os testes Go usam as mesmas fixtures válidas/inválidas do pacote `video-schema`, e um teste falha se a cópia embutida divergir do schema canônico.
- **Erros de união filtrados pelo discriminador `type`**: para uma cena `title` inválida, só os erros do ramo `title` são reportados (mensagens precisas para o usuário e para o LLM).
