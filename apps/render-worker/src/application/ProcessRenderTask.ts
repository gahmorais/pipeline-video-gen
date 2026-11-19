import { isRetryable } from "../domain/errors";
import type { ClaimedRenderTask, JobGateway, RenderTaskQueue } from "../ports";
import type { RenderVideo } from "./RenderVideo";

export type ProcessOutcome =
  | { kind: "completed"; outputKey: string; reused: boolean }
  | { kind: "stale"; reason: string }
  | { kind: "retry"; delayMs: number; error: string }
  | { kind: "failed"; error: string };

export interface ProcessRenderTaskDeps {
  queue: RenderTaskQueue;
  gateway: JobGateway;
  renderVideo: Pick<RenderVideo, "execute">;
  heartbeatMs: number;
  retryBaseMs: number;
  retryMaxMs: number;
}

/**
 * Use case: processar uma tarefa de render reservada na fila.
 *
 * 1. Avisa a API (start) e recebe o storyboard — a API decide se o render ainda é o atual.
 * 2. Renderiza (RenderVideo), mantendo a reserva viva com heartbeats.
 * 3. Reporta o resultado à API e confirma a tarefa na fila.
 *
 * Falhas retentáveis voltam para a fila com backoff (o job continua RENDERING);
 * falhas definitivas ou tentativas esgotadas marcam o job como FAILED na API.
 */
export class ProcessRenderTask {
  constructor(private readonly deps: ProcessRenderTaskDeps) {}

  async execute(task: ClaimedRenderTask): Promise<ProcessOutcome> {
    const { queue, gateway, renderVideo } = this.deps;

    let started;
    try {
      started = await gateway.startRender(task.jobId, task.renderId);
    } catch (err) {
      return this.handleFailure(task, err);
    }
    if (started.kind === "stale") {
      await queue.complete(task.id);
      return started;
    }

    const heartbeat = setInterval(() => {
      queue.heartbeat(task.id).catch(() => undefined);
    }, this.deps.heartbeatMs);

    try {
      const result = await renderVideo.execute({
        jobId: task.jobId,
        renderId: task.renderId,
        templateId: started.job.templateId,
        templateVersion: started.job.templateVersion,
        storyboard: started.job.storyboard,
      });
      await gateway.completeRender(task.jobId, task.renderId, result.outputKey);
      await queue.complete(task.id);
      return { kind: "completed", outputKey: result.outputKey, reused: result.reused };
    } catch (err) {
      return this.handleFailure(task, err);
    } finally {
      clearInterval(heartbeat);
    }
  }

  private async handleFailure(task: ClaimedRenderTask, err: unknown): Promise<ProcessOutcome> {
    const { queue, gateway } = this.deps;
    const error = describe(err);

    if (isRetryable(err) && task.attempts < task.maxAttempts) {
      const delayMs = Math.min(this.deps.retryMaxMs, this.deps.retryBaseMs * 2 ** (task.attempts - 1));
      const { dead } = await queue.fail(task.id, error, delayMs);
      if (!dead) {
        return { kind: "retry", delayMs, error };
      }
    } else {
      await queue.kill(task.id, error);
    }
    // Falha definitiva: a API move o job para FAILED (o usuário pode fazer retry).
    await gateway.failRender(task.jobId, task.renderId, error).catch(() => undefined);
    return { kind: "failed", error };
  }
}

function describe(err: unknown): string {
  if (err instanceof Error) {
    const cause = err.cause instanceof Error ? `: ${err.cause.message}` : "";
    return `${err.message}${cause}`.slice(0, 500);
  }
  return String(err).slice(0, 500);
}
