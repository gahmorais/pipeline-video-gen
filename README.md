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

Sem chave de LLM, use `STORYBOARD_PROVIDER=fixture` e `STORYBOARD_FIXTURE=../../packages/video-schema/fixtures/valid/promo-basic.json` na API.

## Fluxo via API

```bash
API=http://localhost:8080
ID=$(curl -s -X POST $API/videos -d '{"prompt":"ofertas de leitores de código de barras","templateId":"promo"}' | jq -r .id)
curl -N $API/videos/$ID/events                                     # acompanha o estado (SSE)
curl -X PATCH $API/videos/$ID/storyboard -d @meu-roteiro.json       # opcional: editar o roteiro
curl -X POST -H "Idempotency-Key: $(uuidgen | tr A-Z a-z)" $API/videos/$ID/render
curl -o video.mp4 "$API/videos/$ID/media"                           # após COMPLETED
```

## Testes

```bash
pnpm test                                   # TS (unitários)
(cd apps/api && go test ./...)              # Go (unitários)
# Integração (Postgres real — APAGA as tabelas do banco apontado):
DATABASE_URL=postgres://videogen:videogen@localhost:5432/videogen?sslmode=disable go test ./... # em apps/api
DATABASE_URL=postgres://videogen:videogen@localhost:5432/videogen pnpm --filter @videogen/render-worker test:integration
```
