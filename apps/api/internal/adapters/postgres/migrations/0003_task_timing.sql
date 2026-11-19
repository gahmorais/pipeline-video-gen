-- Marcos de tempo da fila para métricas (tempo em fila, duração de processamento).
ALTER TABLE task_queue
    ADD COLUMN first_claimed_at TIMESTAMPTZ,
    ADD COLUMN last_claimed_at  TIMESTAMPTZ,
    ADD COLUMN finished_at      TIMESTAMPTZ;

CREATE INDEX task_queue_finished_idx ON task_queue (finished_at) WHERE status = 'done';
