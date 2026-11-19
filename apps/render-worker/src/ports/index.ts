import type { ValidationIssue } from "../domain/errors";

/** Porta para o motor de render (Remotion local hoje, Remotion Lambda no futuro). */
export interface Renderer {
  render(input: RendererInput): Promise<RendererOutput>;
}

export interface RendererInput {
  compositionId: string;
  inputProps: Record<string, unknown>;
  /** Caminho local onde o arquivo deve ser escrito. */
  outputPath: string;
  /** Progresso de 0 a 1. */
  onProgress: (progress: number) => void;
}

export interface RendererOutput {
  durationInFrames: number;
}

/**
 * Porta de armazenamento de mídia. A escrita é em duas fases (target → commit)
 * para que um arquivo só "exista" depois de completo — base da idempotência.
 */
export interface MediaStorage {
  exists(key: string): Promise<boolean>;
  createWriteTarget(key: string): Promise<WriteTarget>;
}

export interface WriteTarget {
  /** Caminho local temporário onde o renderer escreve. */
  readonly localPath: string;
  commit(): Promise<{ sizeBytes: number }>;
  abort(): Promise<void>;
}

export interface ProgressReporter {
  report(jobId: string, renderId: string, progress: number): Promise<void>;
}

/** Tarefa de render reservada na fila. */
export interface ClaimedRenderTask {
  id: number;
  jobId: string;
  renderId: string;
  attempts: number;
  maxAttempts: number;
}

/** Lado consumidor da fila de render (Postgres hoje, NATS no futuro). */
export interface RenderTaskQueue {
  claim(workerId: string): Promise<ClaimedRenderTask | null>;
  complete(taskId: number): Promise<void>;
  /** Devolve à fila após `delayMs`; `dead=true` quando esgotou as tentativas. */
  fail(taskId: number, error: string, delayMs: number): Promise<{ dead: boolean }>;
  /** Marca como morta sem novas tentativas (erro não retentável). */
  kill(taskId: number, error: string): Promise<void>;
  heartbeat(taskId: number): Promise<void>;
  /** Resolve quando pode haver trabalho novo (notificação) ou após `timeoutMs`. */
  waitForWork(timeoutMs: number, signal?: AbortSignal): Promise<void>;
  close(): Promise<void>;
}

/** Dados do job necessários para renderizar, devolvidos pela API ao iniciar o render. */
export interface RenderJobSnapshot {
  templateId: string;
  templateVersion: number;
  storyboard: unknown;
}

export type StartRenderResult =
  | { kind: "started"; job: RenderJobSnapshot }
  /** O render não é mais o atual (job editado, novo render pedido, job falhou...). */
  | { kind: "stale"; reason: string };

/**
 * Porta para a API, dona do estado do VideoJob (regra 1 do CLAUDE.md):
 * o worker nunca altera o job diretamente, só reporta.
 */
export interface JobGateway {
  startRender(jobId: string, renderId: string): Promise<StartRenderResult>;
  completeRender(jobId: string, renderId: string, outputKey: string): Promise<void>;
  failRender(jobId: string, renderId: string, reason: string): Promise<void>;
}

/** Validação do storyboard contra o schema do template. */
export interface StoryboardValidator {
  validate(templateId: string, templateVersion: number, storyboard: unknown): StoryboardValidation;
}

export type StoryboardValidation =
  | { valid: true; compositionId: string; inputProps: Record<string, unknown> }
  | { valid: false; reason: string; issues: ValidationIssue[] };
