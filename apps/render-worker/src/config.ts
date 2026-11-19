import { resolve } from "node:path";
import { DEFAULT_REMOTION_CONFIG, type RemotionLocalRendererConfig } from "./adapters/remotion-local/RemotionLocalRenderer";

export interface WorkerConfig {
  mediaDir: string;
  remotion: RemotionLocalRendererConfig;
}

function intFromEnv(env: NodeJS.ProcessEnv, name: string, fallback: number): number {
  const raw = env[name];
  if (raw === undefined || raw === "") {
    return fallback;
  }
  const value = Number.parseInt(raw, 10);
  if (!Number.isInteger(value) || value <= 0) {
    throw new Error(`${name} deve ser inteiro positivo, recebido "${raw}"`);
  }
  return value;
}

export function loadConfig(env: NodeJS.ProcessEnv = process.env): WorkerConfig {
  return {
    mediaDir: resolve(env.MEDIA_DIR ?? "media"),
    remotion: {
      concurrency: intFromEnv(env, "RENDER_CONCURRENCY", DEFAULT_REMOTION_CONFIG.concurrency),
      delayRenderTimeoutMs: intFromEnv(env, "RENDER_DELAY_TIMEOUT_MS", DEFAULT_REMOTION_CONFIG.delayRenderTimeoutMs),
      jobTimeoutMs: intFromEnv(env, "RENDER_JOB_TIMEOUT_MS", DEFAULT_REMOTION_CONFIG.jobTimeoutMs),
      browserExecutable: env.BROWSER_EXECUTABLE || null,
    },
  };
}
