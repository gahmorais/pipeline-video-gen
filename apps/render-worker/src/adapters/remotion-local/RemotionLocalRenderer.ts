import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { bundle } from "@remotion/bundler";
import { makeCancelSignal, renderMedia, selectComposition } from "@remotion/renderer";
import { RenderTimeoutError } from "../../domain/errors";
import type { Renderer, RendererInput, RendererOutput } from "../../ports";

export interface RemotionLocalRendererConfig {
  /** Processos de captura em paralelo. Calibrar com `npx remotion benchmark`. */
  concurrency: number;
  /** Timeout de cada delayRender(). */
  delayRenderTimeoutMs: number;
  /** Timeout global do render de um job. */
  jobTimeoutMs: number;
  /** Chrome local opcional; null = Chrome Headless Shell gerenciado pelo Remotion. */
  browserExecutable: string | null;
}

export const DEFAULT_REMOTION_CONFIG: RemotionLocalRendererConfig = {
  concurrency: 4,
  delayRenderTimeoutMs: 30_000,
  jobTimeoutMs: 15 * 60_000,
  browserExecutable: null,
};

const require = createRequire(import.meta.url);

/**
 * Renderer usando Remotion localmente.
 * O bundle é criado UMA vez (lazy, memoizado) e reutilizado por todos os jobs.
 */
export class RemotionLocalRenderer implements Renderer {
  private serveUrl: Promise<string> | null = null;

  constructor(private readonly config: RemotionLocalRendererConfig = DEFAULT_REMOTION_CONFIG) {}

  /** Pré-aquece o bundle no startup do worker. */
  warmUp(): Promise<string> {
    if (!this.serveUrl) {
      const packageRoot = dirname(require.resolve("@videogen/compositions/package.json"));
      this.serveUrl = bundle({
        entryPoint: join(packageRoot, "src", "index.ts"),
        publicDir: join(packageRoot, "public"),
      }).catch((err: unknown) => {
        this.serveUrl = null;
        throw err;
      });
    }
    return this.serveUrl;
  }

  async render(input: RendererInput): Promise<RendererOutput> {
    const serveUrl = await this.warmUp();
    const { browserExecutable } = this.config;
    const chromeMode = browserExecutable ? "chrome-for-testing" : "headless-shell";

    const composition = await selectComposition({
      serveUrl,
      id: input.compositionId,
      inputProps: input.inputProps,
      browserExecutable,
      chromeMode,
      timeoutInMilliseconds: this.config.delayRenderTimeoutMs,
      logLevel: "warn",
    });

    const { cancelSignal, cancel } = makeCancelSignal();
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      cancel();
    }, this.config.jobTimeoutMs);

    try {
      await renderMedia({
        composition,
        serveUrl,
        codec: "h264",
        outputLocation: input.outputPath,
        inputProps: input.inputProps,
        concurrency: this.config.concurrency,
        timeoutInMilliseconds: this.config.delayRenderTimeoutMs,
        browserExecutable,
        chromeMode,
        cancelSignal,
        overwrite: true,
        logLevel: "warn",
        onProgress: ({ progress }) => input.onProgress(progress),
      });
    } catch (err) {
      if (timedOut) {
        throw new RenderTimeoutError(`render excedeu ${this.config.jobTimeoutMs} ms`, { cause: err });
      }
      throw err;
    } finally {
      clearTimeout(timer);
    }

    return { durationInFrames: composition.durationInFrames };
  }
}
