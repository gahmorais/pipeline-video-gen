import { isRetryable } from "../domain/errors";

export interface RetryPolicy {
  maxAttempts: number;
  baseDelayMs: number;
  maxDelayMs: number;
}

export const DEFAULT_RETRY_POLICY: RetryPolicy = {
  maxAttempts: 3,
  baseDelayMs: 2_000,
  maxDelayMs: 30_000,
};

export function backoffDelay(attempt: number, policy: RetryPolicy): number {
  return Math.min(policy.maxDelayMs, policy.baseDelayMs * 2 ** (attempt - 1));
}

/**
 * Executa `fn` com backoff exponencial. Erros não-retentáveis (ex.: storyboard inválido)
 * são propagados imediatamente. `sleep` é injetável para testes determinísticos.
 */
export async function withRetry<T>(
  fn: (attempt: number) => Promise<T>,
  policy: RetryPolicy = DEFAULT_RETRY_POLICY,
  sleep: (ms: number) => Promise<void> = (ms) => new Promise((r) => setTimeout(r, ms)),
  onRetry?: (err: unknown, attempt: number, delayMs: number) => void,
): Promise<T> {
  let attempt = 1;
  for (;;) {
    try {
      return await fn(attempt);
    } catch (err) {
      if (!isRetryable(err) || attempt >= policy.maxAttempts) {
        throw err;
      }
      const delay = backoffDelay(attempt, policy);
      onRetry?.(err, attempt, delay);
      await sleep(delay);
      attempt += 1;
    }
  }
}
