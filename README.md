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
```
