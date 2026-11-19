# pipeline-video-gen

Pipeline local de geração de vídeo: prompt → LLM gera roteiro (JSON) → preview no Remotion Player → render com Remotion.

- Arquitetura e regras: [`CLAUDE.md`](./CLAUDE.md)
- Plano de implementação: [`docs/PLAN.md`](./docs/PLAN.md)

## Pré-requisitos

- Node.js >= 20 e pnpm
- Go >= 1.24
- Docker (apenas para o Postgres)

## Início rápido

```bash
pnpm install
docker compose up -d postgres

# API (Go) — também roda o worker de roteiro (LLM)
cd apps/api
cp .env.example .env   # defina INTERNAL_TOKEN e as chaves de LLM
set -a && source .env && set +a
go run ./cmd/api

# Render worker (Node, nativo no macOS) — em outro terminal
cd apps/render-worker
cp .env.example .env   # mesmo INTERNAL_TOKEN da API
set -a && source .env && set +a
pnpm worker
```

Frontend (Next.js) — em outro terminal:

```bash
cd apps/web
pnpm dev   # http://localhost:3000 (NEXT_PUBLIC_API_URL padrão: http://localhost:8080)
```

Sem chave de LLM, use `STORYBOARD_PROVIDER=fixture` e `STORYBOARD_FIXTURES=promo=../../packages/video-schema/fixtures/valid/promo-basic.json,app-promo=../../packages/video-schema/fixtures/app-promo/valid/basic.json` na API.

## Templates

| Template | Formato | Cenas | Uso |
|---|---|---|---|
| `promo` v1 | 1920×1080 (16:9) | `title`, `product`, `cta` | Ofertas de varejo |
| `app-promo` v1 | 1080×1920 (9:16) | `title`, `clip` (gameplay), `feature`, `cta` (lojas) + trilha | Divulgação de apps/jogos em Reels, TikTok e Shorts |

Exemplo pronto: roteiro de 60 s do Retro 99 em [`docs/examples`](./docs/examples/README.md), com a lista de assets e como gravar o gameplay.

Novo template = nova versão de schema (`packages/video-schema`) + composição registrada (`packages/compositions/src/registry.ts`) + guia de prompt (`apps/api/internal/adapters/llm/prompt.go`). Versões antigas são mantidas para re-render reprodutível.

## Fluxo via API

```bash
API=http://localhost:8080
ID=$(curl -s -X POST $API/videos -d '{"prompt":"ofertas de leitores de código de barras","templateId":"promo"}' | jq -r .id)
# ou templateId "app-promo" para vídeo vertical de divulgação de app
curl -N $API/videos/$ID/events                                     # acompanha o estado (SSE)
curl -X PATCH $API/videos/$ID/storyboard -d @meu-roteiro.json       # opcional: editar o roteiro
curl -X POST -H "Idempotency-Key: $(uuidgen | tr A-Z a-z)" $API/videos/$ID/render
curl -o video.mp4 "$API/videos/$ID/media"                           # após COMPLETED
```

## Operação

- `GET /metrics?window=24h` — jobs por status, tempo em fila e de processamento por tipo de tarefa, segundos de render por segundo de vídeo (base do ETA na UI), taxa de falha por template e consumo/custo do LLM por modelo.
- Logs estruturados com `job_id` na API (`slog`, JSON) e no render worker (`pino`).
- Limpeza automática de `./media` (a cada hora): parciais com mais de 24 h e vídeos substituídos com mais de `MEDIA_ORPHAN_RETENTION`. O vídeo atual de cada job nunca é apagado.
- Rate limit por IP em `POST /videos` e `POST /videos/{id}/render` (429 com `Retry-After`).

## Testes

```bash
pnpm test                                   # TS (unitários)
(cd apps/api && go test ./...)              # Go (unitários)
# Integração (Postgres real — APAGA as tabelas do banco apontado):
DATABASE_URL=postgres://videogen:videogen@localhost:5432/videogen?sslmode=disable go test ./... # em apps/api
DATABASE_URL=postgres://videogen:videogen@localhost:5432/videogen pnpm --filter @videogen/render-worker test:integration
```
