import { RenderFailedError } from "../../domain/errors";
import type { JobGateway, ProgressReporter, StartRenderResult } from "../../ports";

export interface HttpJobGatewayConfig {
  baseUrl: string;
  internalToken: string;
  timeoutMs: number;
}

interface ProblemJson {
  type?: string;
  detail?: string;
}

interface VideoDto {
  template: { id: string; version: number };
  storyboard: unknown;
}

/** Tipos de erro da API que significam "este render não é mais o atual". */
const STALE_TYPES = new Set(["render-mismatch", "invalid-transition", "not-found"]);

/**
 * Adapter HTTP para os endpoints internos da API (/internal/videos/{id}/renders/{renderId}/...).
 * Também implementa ProgressReporter.
 */
export class HttpJobGateway implements JobGateway, ProgressReporter {
  constructor(
    private readonly cfg: HttpJobGatewayConfig,
    private readonly fetchImpl: typeof fetch = fetch,
  ) {}

  async startRender(jobId: string, renderId: string): Promise<StartRenderResult> {
    const res = await this.post(jobId, renderId, "start");
    if (res.ok) {
      const dto = (await res.json()) as VideoDto;
      return {
        kind: "started",
        job: { templateId: dto.template.id, templateVersion: dto.template.version, storyboard: dto.storyboard },
      };
    }
    const problem = await readProblem(res);
    if (problem.type && STALE_TYPES.has(problem.type)) {
      return { kind: "stale", reason: problem.detail ?? problem.type };
    }
    throw this.httpError("start", res.status, problem);
  }

  async completeRender(jobId: string, renderId: string, outputKey: string): Promise<void> {
    const res = await this.post(jobId, renderId, "complete", { outputKey });
    if (!res.ok) {
      throw this.httpError("complete", res.status, await readProblem(res));
    }
  }

  async failRender(jobId: string, renderId: string, reason: string): Promise<void> {
    const res = await this.post(jobId, renderId, "fail", { reason });
    if (!res.ok) {
      throw this.httpError("fail", res.status, await readProblem(res));
    }
  }

  async report(jobId: string, renderId: string, progress: number): Promise<void> {
    const res = await this.post(jobId, renderId, "progress", { progress });
    if (!res.ok) {
      throw this.httpError("progress", res.status, await readProblem(res));
    }
  }

  private post(jobId: string, renderId: string, action: string, body?: unknown): Promise<Response> {
    const url = `${this.cfg.baseUrl}/internal/videos/${encodeURIComponent(jobId)}/renders/${encodeURIComponent(renderId)}/${action}`;
    return this.fetchImpl(url, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${this.cfg.internalToken}`,
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      body: body === undefined ? null : JSON.stringify(body),
      signal: AbortSignal.timeout(this.cfg.timeoutMs),
    });
  }

  private httpError(action: string, status: number, problem: ProblemJson): RenderFailedError {
    return new RenderFailedError(`api ${action}: HTTP ${status} ${problem.type ?? ""} ${problem.detail ?? ""}`.trim());
  }
}

async function readProblem(res: Response): Promise<ProblemJson> {
  try {
    return (await res.json()) as ProblemJson;
  } catch {
    return {};
  }
}
