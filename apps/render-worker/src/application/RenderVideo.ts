import { InvalidRenderRequestError, RenderFailedError, RenderWorkerError } from "../domain/errors";
import { isValidId, outputKeyFor, type RenderRequest, type RenderResult } from "../domain/render";
import type { MediaStorage, ProgressReporter, Renderer, StoryboardValidator } from "../ports";
import { createProgressThrottle } from "./progress-throttle";

export interface RenderVideoDeps {
  renderer: Renderer;
  storage: MediaStorage;
  progress: ProgressReporter;
  validator: StoryboardValidator;
  /** Granularidade dos eventos de progresso (padrão 5%). */
  progressStepPercent?: number;
}

/**
 * Use case: renderizar um storyboard validado para um arquivo de mídia.
 *
 * - Valida ids e storyboard (defesa em profundidade: a API também valida).
 * - Idempotente: se o arquivo do renderId já existe, não renderiza de novo.
 * - Escrita atômica via WriteTarget (commit/abort).
 */
export class RenderVideo {
  constructor(private readonly deps: RenderVideoDeps) {}

  async execute(request: RenderRequest): Promise<RenderResult> {
    const { renderer, storage, progress, validator } = this.deps;

    if (!isValidId(request.jobId) || !isValidId(request.renderId)) {
      throw new InvalidRenderRequestError("jobId/renderId inválidos");
    }

    const validation = validator.validate(
      request.templateId,
      request.templateVersion,
      request.storyboard,
    );
    if (!validation.valid) {
      throw new InvalidRenderRequestError(validation.reason, validation.issues);
    }

    const outputKey = outputKeyFor(request.jobId, request.renderId);

    if (await storage.exists(outputKey)) {
      await progress.report(request.jobId, request.renderId, 1);
      return { jobId: request.jobId, renderId: request.renderId, outputKey, sizeBytes: 0, reused: true };
    }

    const pending: Promise<void>[] = [];
    const onProgress = createProgressThrottle((p) => {
      // Falha ao reportar progresso não deve derrubar o render.
      pending.push(progress.report(request.jobId, request.renderId, p).catch(() => undefined));
    }, this.deps.progressStepPercent);

    const target = await storage.createWriteTarget(outputKey);
    try {
      await renderer.render({
        compositionId: validation.compositionId,
        inputProps: validation.inputProps,
        outputPath: target.localPath,
        onProgress,
      });
      const { sizeBytes } = await target.commit();
      onProgress(1);
      await Promise.all(pending);
      return { jobId: request.jobId, renderId: request.renderId, outputKey, sizeBytes, reused: false };
    } catch (err) {
      await target.abort().catch(() => undefined);
      if (err instanceof RenderWorkerError) {
        throw err;
      }
      throw new RenderFailedError(`falha ao renderizar job ${request.jobId}`, { cause: err });
    }
  }
}
