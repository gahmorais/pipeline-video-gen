import type { Logger } from "pino";
import type { RenderTaskQueue } from "../ports";
import type { ProcessRenderTask } from "./ProcessRenderTask";

export interface WorkerLoopDeps {
  queue: RenderTaskQueue;
  process: Pick<ProcessRenderTask, "execute">;
  logger: Logger;
  workerId: string;
  pollIntervalMs: number;
  errorBackoffMs: number;
}

/**
 * Loop do worker: um render por vez (regra 5 do CLAUDE.md — o paralelismo é
 * interno ao Remotion). Ao receber o sinal de parada, termina o render em andamento.
 */
export class WorkerLoop {
  constructor(private readonly deps: WorkerLoopDeps) {}

  async run(signal: AbortSignal): Promise<void> {
    const { queue, process, logger, workerId } = this.deps;
    while (!signal.aborted) {
      try {
        const task = await queue.claim(workerId);
        if (!task) {
          await queue.waitForWork(this.deps.pollIntervalMs, signal);
          continue;
        }
        const log = logger.child({ job_id: task.jobId, render_id: task.renderId, task_id: task.id, attempt: task.attempts });
        log.info("render iniciado");
        const startedAt = performance.now();
        const outcome = await process.execute(task);
        log.info({ outcome: outcome.kind, elapsed_ms: Math.round(performance.now() - startedAt), ...outcome }, "render finalizado");
      } catch (err) {
        logger.error({ err }, "erro no loop do worker");
        await sleep(this.deps.errorBackoffMs, signal);
      }
    }
  }
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener(
      "abort",
      () => {
        clearTimeout(timer);
        resolve();
      },
      { once: true },
    );
  });
}
