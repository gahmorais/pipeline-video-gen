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

export interface QueueWorkerConfig {
  databaseUrl: string;
  apiUrl: string;
  internalToken: string;
  workerId: string;
  pollIntervalMs: number;
  heartbeatMs: number;
  retryBaseMs: number;
  retryMaxMs: number;
  apiTimeoutMs: number;
}

/** Configuração do modo consumidor de fila (comando `worker`). */
export function loadQueueWorkerConfig(env: NodeJS.ProcessEnv = process.env, hostname = "local"): QueueWorkerConfig {
  const missing = ["DATABASE_URL", "API_URL", "INTERNAL_TOKEN"].filter((k) => !env[k]);
  if (missing.length > 0) {
    throw new Error(`variáveis obrigatórias ausentes: ${missing.join(", ")}`);
  }
  return {
    databaseUrl: env.DATABASE_URL as string,
    apiUrl: (env.API_URL as string).replace(/\/+$/, ""),
    internalToken: env.INTERNAL_TOKEN as string,
    workerId: env.WORKER_ID || `render@${hostname}`,
    pollIntervalMs: intFromEnv(env, "POLL_INTERVAL_MS", 5_000),
    heartbeatMs: intFromEnv(env, "HEARTBEAT_MS", 30_000),
    retryBaseMs: intFromEnv(env, "RETRY_BASE_MS", 10_000),
    retryMaxMs: intFromEnv(env, "RETRY_MAX_MS", 120_000),
    apiTimeoutMs: intFromEnv(env, "API_TIMEOUT_MS", 10_000),
  };
}
