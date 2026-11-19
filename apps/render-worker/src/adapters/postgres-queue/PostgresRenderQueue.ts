import pg from "pg";
import type { ClaimedRenderTask, RenderTaskQueue } from "../../ports";

const CHANNEL = "task_queue_render";

/**
 * Fila de render sobre a tabela task_queue (contrato compartilhado com a API Go).
 * Reserva com SELECT ... FOR UPDATE SKIP LOCKED e acorda via LISTEN/NOTIFY.
 * Só toca na tabela da fila — nunca em video_jobs (regra 1 do CLAUDE.md).
 */
export class PostgresRenderQueue implements RenderTaskQueue {
  private listener: pg.Client | null = null;
  private wakeups: Array<() => void> = [];
  private pendingNotification = false;

  constructor(
    private readonly pool: pg.Pool,
    private readonly connectionString: string,
  ) {}

  /** Abre a conexão dedicada ao LISTEN. */
  async start(): Promise<void> {
    const client = new pg.Client({ connectionString: this.connectionString });
    await client.connect();
    client.on("notification", () => this.wake());
    client.on("error", () => {
      // Conexão perdida: o polling continua garantindo o progresso.
      this.listener = null;
    });
    await client.query(`LISTEN ${CHANNEL}`);
    this.listener = client;
  }

  async claim(workerId: string): Promise<ClaimedRenderTask | null> {
    const { rows } = await this.pool.query<{
      id: string;
      job_id: string;
      render_id: string;
      attempts: number;
      max_attempts: number;
    }>(
      `UPDATE task_queue SET status = 'running', locked_at = now(), locked_by = $1,
              attempts = attempts + 1, updated_at = now()
       WHERE id = (
         SELECT id FROM task_queue
         WHERE kind = 'render' AND status = 'pending' AND run_after <= now()
         ORDER BY run_after, id
         FOR UPDATE SKIP LOCKED
         LIMIT 1)
       RETURNING id, job_id, render_id, attempts, max_attempts`,
      [workerId],
    );
    const row = rows[0];
    if (!row) {
      return null;
    }
    return {
      id: Number(row.id),
      jobId: row.job_id,
      renderId: row.render_id,
      attempts: row.attempts,
      maxAttempts: row.max_attempts,
    };
  }

  async complete(taskId: number): Promise<void> {
    await this.expectOne(
      `UPDATE task_queue SET status = 'done', locked_at = NULL, updated_at = now()
       WHERE id = $1 AND status = 'running'`,
      [taskId],
    );
  }

  async fail(taskId: number, error: string, delayMs: number): Promise<{ dead: boolean }> {
    const { rows } = await this.pool.query<{ status: string }>(
      `UPDATE task_queue SET
         status = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'pending' END,
         run_after = now() + make_interval(secs => $3::float8 / 1000),
         locked_at = NULL, locked_by = NULL, last_error = $2, updated_at = now()
       WHERE id = $1 AND status = 'running'
       RETURNING status`,
      [taskId, error, delayMs],
    );
    if (!rows[0]) {
      throw new Error(`tarefa ${taskId} não está em execução`);
    }
    return { dead: rows[0].status === "dead" };
  }

  async kill(taskId: number, error: string): Promise<void> {
    await this.expectOne(
      `UPDATE task_queue SET status = 'dead', locked_at = NULL, locked_by = NULL,
              last_error = $2, updated_at = now()
       WHERE id = $1 AND status = 'running'`,
      [taskId, error],
    );
  }

  async heartbeat(taskId: number): Promise<void> {
    await this.expectOne(
      `UPDATE task_queue SET locked_at = now(), updated_at = now() WHERE id = $1 AND status = 'running'`,
      [taskId],
    );
  }

  waitForWork(timeoutMs: number, signal?: AbortSignal): Promise<void> {
    if (this.pendingNotification) {
      this.pendingNotification = false;
      return Promise.resolve();
    }
    return new Promise((resolve) => {
      const done = () => {
        clearTimeout(timer);
        signal?.removeEventListener("abort", done);
        this.wakeups = this.wakeups.filter((w) => w !== done);
        resolve();
      };
      const timer = setTimeout(done, timeoutMs);
      signal?.addEventListener("abort", done, { once: true });
      this.wakeups.push(done);
    });
  }

  async close(): Promise<void> {
    this.wakeups.forEach((w) => w());
    await this.listener?.end().catch(() => undefined);
    this.listener = null;
  }

  private wake(): void {
    if (this.wakeups.length === 0) {
      this.pendingNotification = true;
      return;
    }
    this.wakeups.forEach((w) => w());
  }

  private async expectOne(sql: string, params: unknown[]): Promise<void> {
    const res = await this.pool.query(sql, params);
    if (res.rowCount !== 1) {
      throw new Error("tarefa não encontrada ou não está em execução");
    }
  }
}
