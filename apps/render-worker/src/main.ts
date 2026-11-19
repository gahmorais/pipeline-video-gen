/**
 * Composition root do render worker.
 *
 * Uso:
 *   pnpm render <storyboard.json> [--job-id <id>] [--render-id <id>] [--template promo] [--template-version 1]
 *   pnpm worker   (consome a fila de render no Postgres e reporta à API)
 */
import { randomUUID } from "node:crypto";
import { hostname } from "node:os";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { parseArgs } from "node:util";
import pg from "pg";
import { pino } from "pino";
import { HttpJobGateway } from "./adapters/api-gateway/HttpJobGateway";
import { PostgresRenderQueue } from "./adapters/postgres-queue/PostgresRenderQueue";
import { ProcessRenderTask } from "./application/ProcessRenderTask";
import { WorkerLoop } from "./application/WorkerLoop";
import { LoggerProgressReporter } from "./adapters/console/LoggerProgressReporter";
import { LocalFsMediaStorage } from "./adapters/localfs/LocalFsMediaStorage";
import { RemotionLocalRenderer } from "./adapters/remotion-local/RemotionLocalRenderer";
import { VideoSchemaValidator } from "./adapters/schema/VideoSchemaValidator";
import { RenderVideo } from "./application/RenderVideo";
import { withRetry } from "./application/retry";
import { loadConfig, loadQueueWorkerConfig } from "./config";
import { InvalidRenderRequestError } from "./domain/errors";

const logger = pino({ name: "render-worker" });

async function runRenderCommand(argv: string[]): Promise<void> {
  const { values, positionals } = parseArgs({
    args: argv,
    allowPositionals: true,
    options: {
      "job-id": { type: "string" },
      "render-id": { type: "string" },
      template: { type: "string", default: "promo" },
      "template-version": { type: "string", default: "1" },
    },
  });

  const storyboardPath = positionals[0];
  if (!storyboardPath) {
    throw new Error("informe o caminho do storyboard JSON");
  }

  const config = loadConfig();
  const renderer = new RemotionLocalRenderer(config.remotion);
  const useCase = new RenderVideo({
    renderer,
    storage: new LocalFsMediaStorage(config.mediaDir),
    progress: new LoggerProgressReporter(logger),
    validator: new VideoSchemaValidator(),
  });

  const jobId = values["job-id"] ?? `cli-${randomUUID().slice(0, 8)}`;
  const renderId = values["render-id"] ?? randomUUID();
  const log = logger.child({ job_id: jobId, render_id: renderId });

  const storyboard: unknown = JSON.parse(await readFile(resolve(storyboardPath), "utf8"));
  const startedAt = performance.now();

  // No modo CLI o bundle é lazy: o use case valida o storyboard antes, falhando rápido
  // sem pagar o custo do bundle. No modo worker (etapa 10) o bundle é pré-aquecido no startup.
  log.info({ concurrency: config.remotion.concurrency }, "iniciando render");

  const result = await withRetry(
    () =>
      useCase.execute({
        jobId,
        renderId,
        templateId: values.template ?? "promo",
        templateVersion: Number(values["template-version"]),
        storyboard,
      }),
    undefined,
    undefined,
    (err, attempt, delayMs) => log.warn({ err, attempt, delay_ms: delayMs }, "render falhou, nova tentativa"),
  );

  log.info(
    {
      output: new LocalFsMediaStorage(config.mediaDir).pathFor(result.outputKey),
      size_bytes: result.sizeBytes,
      reused: result.reused,
      elapsed_ms: Math.round(performance.now() - startedAt),
    },
    "render concluído",
  );
}

async function runWorkerCommand(): Promise<void> {
  const config = loadConfig();
  const queueConfig = loadQueueWorkerConfig(process.env, hostname());
  const log = logger.child({ worker_id: queueConfig.workerId });

  const pool = new pg.Pool({ connectionString: queueConfig.databaseUrl, max: 3 });
  const queue = new PostgresRenderQueue(pool, queueConfig.databaseUrl);
  const gateway = new HttpJobGateway({
    baseUrl: queueConfig.apiUrl,
    internalToken: queueConfig.internalToken,
    timeoutMs: queueConfig.apiTimeoutMs,
  });
  const renderer = new RemotionLocalRenderer(config.remotion);
  const renderVideo = new RenderVideo({
    renderer,
    storage: new LocalFsMediaStorage(config.mediaDir),
    progress: gateway,
    validator: new VideoSchemaValidator(),
  });
  const loop = new WorkerLoop({
    queue,
    process: new ProcessRenderTask({
      queue,
      gateway,
      renderVideo,
      heartbeatMs: queueConfig.heartbeatMs,
      retryBaseMs: queueConfig.retryBaseMs,
      retryMaxMs: queueConfig.retryMaxMs,
    }),
    logger: log,
    workerId: queueConfig.workerId,
    pollIntervalMs: queueConfig.pollIntervalMs,
    errorBackoffMs: 5_000,
  });

  // Bundle UMA vez no startup (CLAUDE.md: nunca por job).
  log.info({ media_dir: config.mediaDir, concurrency: config.remotion.concurrency }, "pré-aquecendo bundle");
  await renderer.warmUp();
  await queue.start();
  log.info("worker aguardando tarefas de render");

  const controller = new AbortController();
  const stop = (signal: string) => {
    log.info({ signal }, "encerrando após o render em andamento");
    controller.abort();
  };
  process.once("SIGINT", () => stop("SIGINT"));
  process.once("SIGTERM", () => stop("SIGTERM"));

  try {
    await loop.run(controller.signal);
  } finally {
    await queue.close();
    await pool.end();
  }
}

async function main(): Promise<void> {
  const [command, ...rest] = process.argv.slice(2);
  switch (command) {
    case "render":
      await runRenderCommand(rest);
      return;
    case "worker":
      await runWorkerCommand();
      return;
    default:
      throw new Error(`comando desconhecido: ${command ?? "(vazio)"}. Use: render <storyboard.json> | worker`);
  }
}

main().catch((err: unknown) => {
  if (err instanceof InvalidRenderRequestError) {
    logger.error({ issues: err.issues }, err.message);
  } else {
    logger.error({ err }, "falha no render worker");
  }
  process.exit(1);
});
