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
  report(jobId: string, progress: number): Promise<void>;
}

/** Validação do storyboard contra o schema do template. */
export interface StoryboardValidator {
  validate(templateId: string, templateVersion: number, storyboard: unknown): StoryboardValidation;
}

export type StoryboardValidation =
  | { valid: true; compositionId: string; inputProps: Record<string, unknown> }
  | { valid: false; reason: string; issues: ValidationIssue[] };
