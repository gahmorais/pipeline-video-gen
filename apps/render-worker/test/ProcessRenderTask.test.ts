import { pino } from "pino";
import { describe, expect, it, vi } from "vitest";
import { ProcessRenderTask } from "../src/application/ProcessRenderTask";
import { WorkerLoop } from "../src/application/WorkerLoop";
import { InvalidRenderRequestError, RenderFailedError } from "../src/domain/errors";
import type { RenderRequest, RenderResult } from "../src/domain/render";
import type { ClaimedRenderTask, JobGateway, RenderTaskQueue, StartRenderResult } from "../src/ports";

class FakeQueue implements RenderTaskQueue {
  tasks: ClaimedRenderTask[] = [];
  completed: number[] = [];
  failed: Array<{ id: number; delayMs: number }> = [];
  killed: number[] = [];
  heartbeats = 0;
  deadOnFail = false;

  async claim(): Promise<ClaimedRenderTask | null> {
    return this.tasks.shift() ?? null;
  }
  async complete(id: number): Promise<void> {
    this.completed.push(id);
  }
  async fail(id: number, _e: string, delayMs: number): Promise<{ dead: boolean }> {
    this.failed.push({ id, delayMs });
    return { dead: this.deadOnFail };
  }
  async kill(id: number): Promise<void> {
    this.killed.push(id);
  }
  async heartbeat(): Promise<void> {
    this.heartbeats += 1;
  }
  async waitForWork(ms: number, signal?: AbortSignal): Promise<void> {
    await new Promise<void>((r) => {
      const t = setTimeout(r, Math.min(ms, 5));
      signal?.addEventListener("abort", () => {
        clearTimeout(t);
        r();
      });
    });
  }
  async close(): Promise<void> {}
}

class FakeGateway implements JobGateway {
  start: StartRenderResult | Error = {
    kind: "started",
    job: { templateId: "promo", templateVersion: 1, storyboard: { ok: true } },
  };
  completed: string[] = [];
  failed: string[] = [];

  async startRender(): Promise<StartRenderResult> {
    if (this.start instanceof Error) throw this.start;
    return this.start;
  }
  async completeRender(_j: string, _r: string, key: string): Promise<void> {
    this.completed.push(key);
  }
  async failRender(_j: string, _r: string, reason: string): Promise<void> {
    this.failed.push(reason);
  }
}

const task = (attempts = 1, maxAttempts = 3): ClaimedRenderTask => ({
  id: 7,
  jobId: "job-1",
  renderId: "r-1",
  attempts,
  maxAttempts,
});

function setup(render: (req: RenderRequest) => Promise<RenderResult>) {
  const queue = new FakeQueue();
  const gateway = new FakeGateway();
  const calls: RenderRequest[] = [];
  const useCase = new ProcessRenderTask({
    queue,
    gateway,
    renderVideo: {
      execute: async (req) => {
        calls.push(req);
        return render(req);
      },
    },
    heartbeatMs: 5,
    retryBaseMs: 1000,
    retryMaxMs: 3000,
  });
  return { queue, gateway, calls, useCase };
}

const ok = async (req: RenderRequest): Promise<RenderResult> => ({
  jobId: req.jobId,
  renderId: req.renderId,
  outputKey: `${req.jobId}/${req.renderId}.mp4`,
  sizeBytes: 10,
  reused: false,
});

describe("ProcessRenderTask", () => {
  it("renderiza com o storyboard devolvido pela API e confirma", async () => {
    const { queue, gateway, calls, useCase } = setup(ok);
    const outcome = await useCase.execute(task());
    expect(outcome).toEqual({ kind: "completed", outputKey: "job-1/r-1.mp4", reused: false });
    expect(calls[0]).toMatchObject({ templateId: "promo", templateVersion: 1, storyboard: { ok: true } });
    expect(gateway.completed).toEqual(["job-1/r-1.mp4"]);
    expect(queue.completed).toEqual([7]);
  });

  it("descarta tarefa obsoleta sem renderizar", async () => {
    const { queue, gateway, calls, useCase } = setup(ok);
    gateway.start = { kind: "stale", reason: "render-mismatch" };
    expect(await useCase.execute(task())).toEqual({ kind: "stale", reason: "render-mismatch" });
    expect(calls).toHaveLength(0);
    expect(queue.completed).toEqual([7]);
  });

  it("mantém a reserva viva com heartbeats durante o render", async () => {
    const { queue, useCase } = setup(async (req) => {
      await new Promise((r) => setTimeout(r, 30));
      return ok(req);
    });
    await useCase.execute(task());
    expect(queue.heartbeats).toBeGreaterThanOrEqual(2);
  });

  it("falha retentável volta à fila com backoff exponencial limitado", async () => {
    const { queue, gateway, useCase } = setup(async () => {
      throw new RenderFailedError("chrome crashed");
    });
    expect(await useCase.execute(task(1))).toMatchObject({ kind: "retry", delayMs: 1000 });
    expect(await useCase.execute(task(2))).toMatchObject({ kind: "retry", delayMs: 2000 });
    expect(queue.failed.map((f) => f.delayMs)).toEqual([1000, 2000]);
    expect(gateway.failed).toHaveLength(0);
  });

  it("API indisponível no start também é retentável", async () => {
    const { queue, gateway, useCase } = setup(ok);
    gateway.start = new Error("ECONNREFUSED");
    expect(await useCase.execute(task())).toMatchObject({ kind: "retry" });
    expect(queue.failed).toHaveLength(1);
  });

  it("última tentativa marca o job como FAILED na API", async () => {
    const { queue, gateway, useCase } = setup(async () => {
      throw new RenderFailedError("sem memória", { cause: new Error("OOM") });
    });
    const outcome = await useCase.execute(task(3, 3));
    expect(outcome).toEqual({ kind: "failed", error: "sem memória: OOM" });
    expect(queue.killed).toEqual([7]);
    expect(gateway.failed).toEqual(["sem memória: OOM"]);
  });

  it("erro não retentável falha de imediato, sem nova tentativa", async () => {
    const { queue, gateway, useCase } = setup(async () => {
      throw new InvalidRenderRequestError("storyboard inválido");
    });
    expect(await useCase.execute(task(1))).toMatchObject({ kind: "failed" });
    expect(queue.failed).toHaveLength(0);
    expect(queue.killed).toEqual([7]);
    expect(gateway.failed).toEqual(["storyboard inválido"]);
  });

  it("fila decidindo que a tarefa morreu também falha o job", async () => {
    const { queue, gateway, useCase } = setup(async () => {
      throw new RenderFailedError("x");
    });
    queue.deadOnFail = true;
    expect(await useCase.execute(task(1))).toMatchObject({ kind: "failed" });
    expect(gateway.failed).toHaveLength(1);
  });

  it("se a API falhar ao concluir, a tarefa é repetida (render idempotente)", async () => {
    const { queue, gateway, useCase } = setup(ok);
    gateway.completeRender = async () => {
      throw new Error("api fora do ar");
    };
    expect(await useCase.execute(task(1))).toMatchObject({ kind: "retry" });
    expect(queue.completed).toHaveLength(0);
  });
});

describe("WorkerLoop", () => {
  it("processa tarefas uma a uma e para quando sinalizado", async () => {
    const queue = new FakeQueue();
    queue.tasks = [task(), { ...task(), id: 8 }];
    const execute = vi.fn(async () => ({ kind: "completed" as const, outputKey: "k", reused: false }));
    const controller = new AbortController();
    const loop = new WorkerLoop({
      queue,
      process: { execute },
      logger: pino({ level: "silent" }),
      workerId: "w",
      pollIntervalMs: 1000,
      errorBackoffMs: 10,
    });
    const running = loop.run(controller.signal);
    await vi.waitFor(() => expect(execute).toHaveBeenCalledTimes(2));
    controller.abort();
    await running;
  });

  it("sobrevive a erros da fila com backoff", async () => {
    const queue = new FakeQueue();
    let calls = 0;
    queue.claim = async () => {
      calls += 1;
      if (calls === 1) throw new Error("db down");
      return null;
    };
    const controller = new AbortController();
    const loop = new WorkerLoop({
      queue,
      process: { execute: vi.fn() },
      logger: pino({ level: "silent" }),
      workerId: "w",
      pollIntervalMs: 5,
      errorBackoffMs: 5,
    });
    const running = loop.run(controller.signal);
    await vi.waitFor(() => expect(calls).toBeGreaterThanOrEqual(3));
    controller.abort();
    await running;
  });
});
