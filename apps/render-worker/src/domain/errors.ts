/**
 * Erros de domínio. `retryable` orienta a política de retry:
 * entrada inválida nunca melhora com nova tentativa; falha de render pode melhorar.
 */
export abstract class RenderWorkerError extends Error {
  abstract readonly retryable: boolean;
  abstract readonly code: string;

  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = new.target.name;
  }
}

export interface ValidationIssue {
  path: string;
  message: string;
}

export class InvalidRenderRequestError extends RenderWorkerError {
  readonly retryable = false;
  readonly code = "INVALID_REQUEST";

  constructor(
    message: string,
    readonly issues: ValidationIssue[] = [],
  ) {
    super(message);
  }
}

export class RenderFailedError extends RenderWorkerError {
  readonly retryable = true;
  readonly code = "RENDER_FAILED";
}

export class RenderTimeoutError extends RenderWorkerError {
  readonly retryable = true;
  readonly code = "RENDER_TIMEOUT";
}

export function isRetryable(err: unknown): boolean {
  return err instanceof RenderWorkerError ? err.retryable : true;
}
