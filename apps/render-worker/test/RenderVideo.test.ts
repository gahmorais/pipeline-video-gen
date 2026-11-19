import { describe, expect, it } from "vitest";
import { RenderVideo } from "../src/application/RenderVideo";
import { InvalidRenderRequestError, RenderFailedError, RenderTimeoutError } from "../src/domain/errors";
import type { RenderRequest } from "../src/domain/render";
import { FakeProgress, FakeRenderer, FakeStorage, FakeValidator } from "./fakes";

const request: RenderRequest = {
  jobId: "job-1",
  renderId: "render-1",
  templateId: "promo",
  templateVersion: 1,
  storyboard: {},
};

function setup(overrides: Partial<ConstructorParameters<typeof RenderVideo>[0]> = {}) {
  const deps = {
    renderer: new FakeRenderer(),
    storage: new FakeStorage(),
    progress: new FakeProgress(),
    validator: new FakeValidator(),
    ...overrides,
  };
  return { deps, useCase: new RenderVideo(deps) };
}

describe("RenderVideo", () => {
  it("renderiza, faz commit e retorna a chave do arquivo", async () => {
    const { deps, useCase } = setup();
    const result = await useCase.execute(request);

    expect(result).toEqual({
      jobId: "job-1",
      renderId: "render-1",
      outputKey: "job-1/render-1.mp4",
      sizeBytes: 1234,
      reused: false,
    });
    const renderer = deps.renderer as FakeRenderer;
    expect(renderer.calls).toHaveLength(1);
    expect(renderer.calls[0]).toMatchObject({ compositionId: "Promo", inputProps: { ok: true } });
    expect((deps.storage as FakeStorage).committed.has("job-1/render-1.mp4")).toBe(true);
  });

  it("emite progresso em degraus de 5%, monotônico e terminando em 100%", async () => {
    const { deps, useCase } = setup();
    await useCase.execute(request);
    expect((deps.progress as FakeProgress).events).toEqual([0, 0.05, 0.5, 0.95, 1]);
  });

  it("é idempotente: não renderiza se o arquivo do renderId já existe", async () => {
    const { deps, useCase } = setup({ storage: new FakeStorage(["job-1/render-1.mp4"]) });
    const result = await useCase.execute(request);
    expect(result.reused).toBe(true);
    expect((deps.renderer as FakeRenderer).calls).toHaveLength(0);
    expect((deps.progress as FakeProgress).events).toEqual([1]);
  });

  it("rejeita storyboard inválido sem renderizar (erro não retentável)", async () => {
    const { deps, useCase } = setup({
      validator: new FakeValidator({
        valid: false,
        reason: "storyboard inválido",
        issues: [{ path: "scenes", message: "vazio" }],
      }),
    });
    const err = await useCase.execute(request).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(InvalidRenderRequestError);
    expect((err as InvalidRenderRequestError).retryable).toBe(false);
    expect((err as InvalidRenderRequestError).issues).toEqual([{ path: "scenes", message: "vazio" }]);
    expect((deps.renderer as FakeRenderer).calls).toHaveLength(0);
  });

  it.each([
    ["../escape", "render-1"],
    ["job-1", "a/b"],
    ["", "render-1"],
  ])("rejeita ids inseguros (%s, %s)", async (jobId, renderId) => {
    const { useCase } = setup();
    await expect(useCase.execute({ ...request, jobId, renderId })).rejects.toBeInstanceOf(
      InvalidRenderRequestError,
    );
  });

  it("aborta o arquivo parcial e embrulha falhas desconhecidas como RenderFailedError", async () => {
    const storage = new FakeStorage();
    const { useCase } = setup({
      storage,
      renderer: new FakeRenderer(async () => {
        throw new Error("chrome crashed");
      }),
    });
    const err = await useCase.execute(request).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(RenderFailedError);
    expect((err as RenderFailedError).retryable).toBe(true);
    expect((err as Error).cause).toBeInstanceOf(Error);
    expect(storage.aborted).toEqual(["job-1/render-1.mp4"]);
    expect(storage.committed.size).toBe(0);
  });

  it("preserva erros de domínio vindos do renderer (ex.: timeout)", async () => {
    const { useCase } = setup({
      renderer: new FakeRenderer(async () => {
        throw new RenderTimeoutError("timeout");
      }),
    });
    await expect(useCase.execute(request)).rejects.toBeInstanceOf(RenderTimeoutError);
  });

  it("não falha o render se o reporte de progresso falhar", async () => {
    const { useCase } = setup({
      progress: {
        report: async () => {
          throw new Error("api fora do ar");
        },
      },
    });
    await expect(useCase.execute(request)).resolves.toMatchObject({ reused: false });
  });
});
