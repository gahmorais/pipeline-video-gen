-- Estado do agregado VideoJob. Só a API escreve aqui (regra 1 do CLAUDE.md).
CREATE TABLE video_jobs (
    id               TEXT PRIMARY KEY,
    prompt           TEXT NOT NULL,
    template_id      TEXT NOT NULL,
    template_version INTEGER NOT NULL CHECK (template_version > 0),
    status           TEXT NOT NULL CHECK (status IN (
                         'DRAFT', 'GENERATING_STORYBOARD', 'STORYBOARD_READY',
                         'RENDER_QUEUED', 'RENDERING', 'COMPLETED', 'FAILED')),
    storyboard       JSONB,
    render_id        TEXT NOT NULL DEFAULT '',
    render_progress  DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (render_progress BETWEEN 0 AND 1),
    output_key       TEXT NOT NULL DEFAULT '',
    failure_reason   TEXT,
    failure_from     TEXT,
    failed_at        TIMESTAMPTZ,
    retry_count      INTEGER NOT NULL DEFAULT 0,
    version          INTEGER NOT NULL CHECK (version > 0),
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL,
    CHECK ((status = 'FAILED') = (failure_reason IS NOT NULL))
);

CREATE INDEX video_jobs_status_idx ON video_jobs (status);

-- Fila de tarefas. Compartilhada entre a API (produtor), o storyboard worker (Go)
-- e o render worker (Node). Contrato: kind, job_id, render_id, status, locked_at.
CREATE TABLE task_queue (
    id           BIGSERIAL PRIMARY KEY,
    kind         TEXT NOT NULL CHECK (kind IN ('storyboard', 'render')),
    job_id       TEXT NOT NULL REFERENCES video_jobs (id) ON DELETE CASCADE,
    render_id    TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'dead')),
    attempts     INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
    run_after    TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at    TIMESTAMPTZ,
    locked_by    TEXT,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Busca de trabalho disponível (Claim).
CREATE INDEX task_queue_ready_idx ON task_queue (kind, run_after, id) WHERE status = 'pending';
-- Detecção de workers mortos (ReclaimStale).
CREATE INDEX task_queue_running_idx ON task_queue (locked_at) WHERE status = 'running';
-- No máximo uma tarefa ativa por (kind, job, render): enfileirar de novo é idempotente.
CREATE UNIQUE INDEX task_queue_active_uniq ON task_queue (kind, job_id, render_id)
    WHERE status IN ('pending', 'running');
