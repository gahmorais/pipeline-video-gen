/**
 * Integração com Postgres real. Roda só com DATABASE_URL definido.
 * Aplica as migrações da API (contrato compartilhado da tabela task_queue).
 * ATENÇÃO: apaga as tabelas do banco apontado.
 */
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import pg from "pg";
import { afterAll, beforeEach, describe, expect, it } from "vitest";
import { PostgresRenderQueue } from "../../src/adapters/postgres-queue/PostgresRenderQueue";

const url = process.env.DATABASE_URL;
const here = dirname(fileURLToPath(import.meta.url));
const migrationsDir = join(here, "..", "..", "..", "api", "internal", "adapters", "postgres", "migrations");

describe.runIf(Boolean(url))("PostgresRenderQueue", () => {
  const pool = new pg.Pool({ connectionString: url, max: 5 });

  afterAll(async () => {
    await pool.end();
  });

  beforeEach(async () => {
    await pool.query("DROP TABLE IF EXISTS llm_usage, task_queue, video_jobs, schema_migrations CASCADE");
    for (const file of readdirSync(migrationsDir).sort()) {
      await pool.query(readFileSync(join(migrationsDir, file), "utf8"));
    }
  });

  async function insertTask(jobId: string, kind = "render", renderId = "r1"): Promise<void> {
    await pool.query(
      `INSERT INTO video_jobs (id, prompt, template_id, template_version, status, version, created_at, updated_at)
       VALUES ($1, 'p', 'promo', 1, 'RENDER_QUEUED', 1, now(), now()) ON CONFLICT DO NOTHING`,
      [jobId],
    );
    await pool.query(`INSERT INTO task_queue (kind, job_id, render_id) VALUES ($1, $2, $3)`, [kind, jobId, renderId]);
    await pool.query(`SELECT pg_notify('task_queue_' || $1, $2)`, [kind, jobId]);
  }

  it("reserva só tarefas de render, uma vez cada, em ordem", async () => {
    await insertTask("a");
    await insertTask("s", "storyboard", "");
    await insertTask("b");
    const queue = new PostgresRenderQueue(pool, url as string);

    const first = await queue.claim("w");
    const second = await queue.claim("w");
    expect([first?.jobId, second?.jobId]).toEqual(["a", "b"]);
    expect(first).toMatchObject({ renderId: "r1", attempts: 1, maxAttempts: 3 });
    expect(await queue.claim("w")).toBeNull();
  });

  it("reservas concorrentes nunca duplicam (SKIP LOCKED)", async () => {
    for (let i = 0; i < 20; i++) await insertTask(`job-${i}`);
    const queue = new PostgresRenderQueue(pool, url as string);
    const claims = await Promise.all(Array.from({ length: 30 }, () => queue.claim("w")));
    const ids = claims.filter((c) => c !== null).map((c) => c?.id);
    expect(ids).toHaveLength(20);
    expect(new Set(ids).size).toBe(20);
  });

  it("fail com backoff, kill e complete atualizam o estado", async () => {
    await insertTask("a");
    await insertTask("b");
    await insertTask("c");
    const queue = new PostgresRenderQueue(pool, url as string);
    const [a, b, c] = [await queue.claim("w"), await queue.claim("w"), await queue.claim("w")];

    expect(await queue.fail(a!.id, "chrome", 60_000)).toEqual({ dead: false });
    await queue.kill(b!.id, "inválido");
    await queue.complete(c!.id);
    await queue.heartbeat(c!.id).then(
      () => {
        throw new Error("heartbeat em tarefa concluída deveria falhar");
      },
      () => undefined,
    );

    const { rows } = await pool.query(`SELECT job_id, status, last_error FROM task_queue ORDER BY job_id`);
    expect(rows).toEqual([
      { job_id: "a", status: "pending", last_error: "chrome" },
      { job_id: "b", status: "dead", last_error: "inválido" },
      { job_id: "c", status: "done", last_error: null },
    ]);
    expect(await queue.claim("w")).toBeNull(); // "a" está em backoff
  });

  it("acorda com NOTIFY antes do timeout", async () => {
    const queue = new PostgresRenderQueue(pool, url as string);
    await queue.start();
    try {
      const startedAt = Date.now();
      const waiting = queue.waitForWork(10_000);
      setTimeout(() => void insertTask("n"), 50);
      await waiting;
      expect(Date.now() - startedAt).toBeLessThan(3_000);
    } finally {
      await queue.close();
    }
  });
});
