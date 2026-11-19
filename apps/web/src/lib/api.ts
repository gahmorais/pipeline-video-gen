/**
 * Adapter da API (Go). Único ponto do frontend que conhece URLs e formatos HTTP.
 * Os tipos espelham httpapi.VideoDTO.
 */

export type JobStatus =
  | "DRAFT"
  | "GENERATING_STORYBOARD"
  | "STORYBOARD_READY"
  | "RENDER_QUEUED"
  | "RENDERING"
  | "COMPLETED"
  | "FAILED";

export interface VideoDto {
  id: string;
  status: JobStatus;
  prompt: string;
  template: { id: string; version: number };
  storyboard: unknown | null;
  render: { id: string; progress: number } | null;
  mediaUrl?: string;
  failure: { reason: string; from: string; at: string } | null;
  retryCount: number;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface ValidationIssue {
  path: string;
  message: string;
}

/** Erro no formato RFC 9457 devolvido pela API. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly type: string,
    readonly detail: string,
    readonly issues: ValidationIssue[] = [],
  ) {
    super(detail || type || `HTTP ${status}`);
    this.name = "ApiError";
  }
}

export class ApiClient {
  constructor(
    readonly baseUrl: string,
    private readonly fetchImpl: typeof fetch = (...args) => fetch(...args),
  ) {}

  createVideo(prompt: string, templateId = "promo"): Promise<VideoDto> {
    return this.request("POST", "/videos", { prompt, templateId });
  }

  getVideo(id: string): Promise<VideoDto> {
    return this.request("GET", `/videos/${encodeURIComponent(id)}`);
  }

  updateStoryboard(id: string, storyboard: unknown): Promise<VideoDto> {
    return this.request("PATCH", `/videos/${encodeURIComponent(id)}/storyboard`, storyboard);
  }

  /** A idempotency key evita render duplicado em duplo clique ou reenvio. */
  requestRender(id: string, idempotencyKey: string): Promise<VideoDto> {
    return this.request("POST", `/videos/${encodeURIComponent(id)}/render`, undefined, {
      "Idempotency-Key": idempotencyKey,
    });
  }

  retry(id: string): Promise<VideoDto> {
    return this.request("POST", `/videos/${encodeURIComponent(id)}/retry`);
  }

  /** Resumo operacional (usado para o ETA do render). */
  getMetrics(window = "168h"): Promise<{ render: { renders: number; renderSecondsPerVideoSecond: number } }> {
    return this.request("GET", `/metrics?window=${encodeURIComponent(window)}`);
  }

  eventsUrl(id: string): string {
    return `${this.baseUrl}/videos/${encodeURIComponent(id)}/events`;
  }

  /** mediaUrl vem relativo da API; resolve contra a base. */
  absoluteUrl(path: string): string {
    return new URL(path, this.baseUrl).toString();
  }

  private async request<T>(method: string, path: string, body?: unknown, headers: Record<string, string> = {}): Promise<T> {
    const res = await this.fetchImpl(`${this.baseUrl}${path}`, {
      method,
      headers: { ...(body === undefined ? {} : { "Content-Type": "application/json" }), ...headers },
      body: body === undefined ? null : JSON.stringify(body),
    });
    if (!res.ok) {
      let problem: { type?: string; detail?: string; title?: string; issues?: ValidationIssue[] } = {};
      try {
        problem = await res.json();
      } catch {
        // corpo não-JSON
      }
      throw new ApiError(res.status, problem.type ?? "", problem.detail ?? problem.title ?? "", problem.issues ?? []);
    }
    return (await res.json()) as T;
  }
}

export const api = new ApiClient(process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080");

/** Nova idempotency key (crypto.randomUUID está disponível em browsers modernos). */
export function newIdempotencyKey(): string {
  return crypto.randomUUID();
}
