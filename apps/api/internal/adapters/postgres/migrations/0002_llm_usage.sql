-- Métricas de uso do LLM por chamada (custo por job, taxa de falha por modelo).
CREATE TABLE llm_usage (
    id            BIGSERIAL PRIMARY KEY,
    job_id        TEXT NOT NULL REFERENCES video_jobs (id) ON DELETE CASCADE,
    model         TEXT NOT NULL,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd      NUMERIC(12, 6) NOT NULL DEFAULT 0,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    success       BOOLEAN NOT NULL,
    error         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX llm_usage_job_idx ON llm_usage (job_id);
CREATE INDEX llm_usage_created_idx ON llm_usage (created_at);
