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
| 7 | Postgres: repositório + fila | Migrations, `VideoJobRepository`, `JobQueue` (SKIP LOCKED, reclaim de jobs travados, LISTEN/NOTIFY) | Testes de integração contra Postgres real | ✅ |
| 8 | HTTP + SSE | Handlers REST, SSE de eventos por job, endpoints internos para workers, servir MP4 (`http.ServeContent`) | `httptest` + teste e2e da API com Postgres | ✅ |
| 9 | LLM adapters | `GeminiStoryboardGenerator`, `AnthropicStoryboardGenerator`, `FallbackGenerator`, `MeteredGenerator`; storyboard worker | `go test` com servidores HTTP fake simulando os provedores | ✅ (validar com chaves reais) |
| 10 | Render worker conectado | Adapter de fila Postgres + reporter HTTP para a API; loop de consumo | Teste e2e: job criado via API → render → `COMPLETED` | ✅ |
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
- **`lib/pq` + `database/sql`** em vez de `pgx`, e testes de integração com `DATABASE_URL` em vez de testcontainers: ambos sem dependências de `golang.org/x/*` (indisponível no ambiente de desenvolvimento remoto). `lib/pq` suporta `LISTEN/NOTIFY` via `pq.Listener`.
- **Contrato da fila (`task_queue`)** compartilhado por API (produtor), storyboard worker (Go) e render worker (Node): `kind`, `job_id`, `render_id`, `status` (`pending`/`running`/`done`/`dead`), `attempts`/`max_attempts`, `run_after` (backoff), `locked_at`/`locked_by` (reclaim). Índice único parcial impede tarefas ativas duplicadas (enqueue idempotente).
- **NOTIFY dentro da transação**: entregue só no commit — workers nunca acordam para trabalho desfeito. Notificação é só uma dica; workers também fazem polling.
- **HTTP só com stdlib** (`net/http`, padrões de rota do Go 1.22+). Erros no formato RFC 9457 (`application/problem+json`); validação de schema → 422 com `issues` por caminho.
- **Endpoints internos dos workers** (`/internal/videos/{id}/renders/{renderId}/start|progress|complete|fail`) protegidos por `Authorization: Bearer $INTERNAL_TOKEN` (comparação em tempo constante). `complete` confere que o arquivo existe antes de marcar `COMPLETED`.
- **SSE envia snapshots completos** (DTO inteiro) a cada mudança, não deltas: o cliente só substitui o estado. O broker coalesce sinais e o handler relê o job; assina antes da leitura inicial para não perder mudanças. O servidor **não** fecha o stream em estados finais (o `EventSource` reconectaria em loop).
- **Broker em memória**: adequado a uma instância da API (cenário local). Múltiplas instâncias exigiriam fan-out via `LISTEN/NOTIFY`.
- **Mídia via `http.ServeContent`** (Range/seek); `mediaUrl` inclui `?r=<renderId>` para invalidar cache após re-render.
- **Composição dos geradores**: `Fallback(Metered(Gemini), Metered(Anthropic))` — cada provedor medido individualmente (as duas tentativas aparecem em `llm_usage`). O fallback valida a saída do primário contra o schema antes de aceitar.
- **Anthropic**: structured output via ferramenta `submit_storyboard` (`input_schema` = JSON Schema do template) com `tool_choice: auto` (compatível com extended thinking); se vier texto, o JSON é extraído. `cache_control: ephemeral` no system prompt e na ferramenta (prompt caching).
- **Gemini**: `generationConfig.responseMimeType=application/json` + `responseJsonSchema`; chave no header `x-goog-api-key` (nunca na URL). Tokens de "thinking" contados como saída.
- ⚠️ **A validar com chaves reais**: os formatos das APIs foram implementados e testados contra servidores fake; a documentação mais recente do Gemini também mostra `generationConfig.responseFormat`. Confirmar nomes de campo e IDs de modelo (`GEMINI_MODEL`, `ANTHROPIC_MODEL`) na primeira execução real.
- **Provedor `fixture`** (`STORYBOARD_PROVIDER=fixture`): desenvolvimento e testes de ponta a ponta sem custo.
- **Storyboard worker como goroutines da API**: `STORYBOARD_WORKERS` (padrão 2) loops, acordados por `LISTEN/NOTIFY` com polling de segurança. Falha do LLM vira `FAILED` no use case; erros de infraestrutura usam o retry/backoff da fila e, esgotados, marcam o job como `FAILED`.
- **Manutenção da fila** (reclaim de tarefas abandonadas, de qualquer tipo) roda só na API, a cada minuto.
- **Assets permitidos ao LLM** fixos no system prompt (`assets/logo.png`, `assets/product-placeholder.png`) até existir upload de assets.
- **Render worker não lê `video_jobs`**: o endpoint interno `start` devolve o snapshot do job (template + storyboard). O worker só toca na tabela `task_queue` e reporta à API, que continua dona do estado.
- **`ProcessRenderTask`** (use case do worker): `start` na API → render com heartbeats → `complete` na API → `done` na fila. Respostas `render-mismatch`/`invalid-transition`/`not-found` no `start` = tarefa obsoleta (descartada sem renderizar). Falhas retentáveis voltam à fila com backoff (job continua `RENDERING`); falha definitiva ou tentativas esgotadas → `fail` na API (`FAILED`).
- **Retry só na fila** no modo worker (sem `withRetry` em processo), evitando retentativas multiplicadas.
- **Um render por vez por worker** (`WorkerLoop`); SIGINT/SIGTERM aguardam o render em andamento.
- **E2E validado**: API + storyboard worker (fixture) + render worker (Remotion real) → MP4 H.264 1080p30 servido com Range.
