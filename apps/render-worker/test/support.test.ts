import { mkdtemp, readdir, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { LocalFsMediaStorage } from "../src/adapters/localfs/LocalFsMediaStorage";
import { VideoSchemaValidator } from "../src/adapters/schema/VideoSchemaValidator";
import { createProgressThrottle } from "../src/application/progress-throttle";
import { backoffDelay, withRetry } from "../src/application/retry";
import { loadConfig } from "../src/config";
import { InvalidRenderRequestError, RenderFailedError } from "../src/domain/errors";

const require = createRequire(import.meta.url);

describe("LocalFsMediaStorage", () => {
  it("só considera o arquivo existente após commit (escrita atômica)", async () => {
    const root = await mkdtemp(join(tmpdir(), "media-"));
    const storage = new LocalFsMediaStorage(root);
    const target = await storage.createWriteTarget("job/render.mp4");

    await writeFile(target.localPath, "conteúdo");
    expect(await storage.exists("job/render.mp4")).toBe(false);

    const { sizeBytes } = await target.commit();
    expect(sizeBytes).toBe(9);
    expect(await storage.exists("job/render.mp4")).toBe(true);
    expect(await readdir(join(root, "job"))).toEqual(["render.mp4"]);
  });

  it("abort remove o parcial", async () => {
    const root = await mkdtemp(join(tmpdir(), "media-"));
    const storage = new LocalFsMediaStorage(root);
    const target = await storage.createWriteTarget("job/r.mp4");
    await writeFile(target.localPath, "x");
    await target.abort();
    expect(await readdir(join(root, "job"))).toEqual([]);
  });

  it("rejeita commit de arquivo vazio", async () => {
    const root = await mkdtemp(join(tmpdir(), "media-"));
    const target = await new LocalFsMediaStorage(root).createWriteTarget("job/r.mp4");
    await writeFile(target.localPath, "");
    await expect(target.commit()).rejects.toThrow(/vazio/);
  });

  it("bloqueia path traversal", () => {
    const storage = new LocalFsMediaStorage("/tmp/media-root");
    expect(() => storage.pathFor("../etc/passwd")).toThrow();
    expect(() => storage.pathFor("/etc/passwd")).toThrow();
  });
});

describe("VideoSchemaValidator", () => {
  const validator = new VideoSchemaValidator();
  const fixture: unknown = JSON.parse(
    readFileSync(require.resolve("@videogen/video-schema/fixtures/valid/promo-basic.json"), "utf8"),
  );

  it("aceita fixture válida e retorna o compositionId do template", () => {
    expect(validator.validate("promo", 1, fixture)).toMatchObject({ valid: true, compositionId: "Promo" });
  });

  it("rejeita template desconhecido, versão divergente e storyboard inválido", () => {
    expect(validator.validate("nope", 1, fixture)).toMatchObject({ valid: false });
    expect(validator.validate("promo", 2, fixture)).toMatchObject({ valid: false });
    expect(validator.validate("promo", 1, { schemaVersion: 1 })).toMatchObject({ valid: false });
  });
});

describe("createProgressThrottle", () => {
  it("ignora regressões e valores fora de [0,1]", () => {
    const events: number[] = [];
    const throttle = createProgressThrottle((p) => events.push(p), 25);
    [-1, 0.1, 0.3, 0.2, 0.55, 2].forEach(throttle);
    expect(events).toEqual([0, 0.25, 0.5, 1]);
  });
});

describe("withRetry", () => {
  const policy = { maxAttempts: 3, baseDelayMs: 100, maxDelayMs: 250 };

  it("calcula backoff exponencial com teto", () => {
    expect([1, 2, 3, 4].map((a) => backoffDelay(a, policy))).toEqual([100, 200, 250, 250]);
  });

  it("tenta novamente erros retentáveis até sucesso", async () => {
    const sleeps: number[] = [];
    let calls = 0;
    const result = await withRetry(
      async () => {
        calls += 1;
        if (calls < 3) throw new RenderFailedError("falhou");
        return "ok";
      },
      policy,
      async (ms) => {
        sleeps.push(ms);
      },
    );
    expect(result).toBe("ok");
    expect(sleeps).toEqual([100, 200]);
  });

  it("desiste após maxAttempts", async () => {
    let calls = 0;
    await expect(
      withRetry(
        async () => {
          calls += 1;
          throw new RenderFailedError("sempre");
        },
        policy,
        async () => undefined,
      ),
    ).rejects.toBeInstanceOf(RenderFailedError);
    expect(calls).toBe(3);
  });

  it("não repete erros não retentáveis", async () => {
    let calls = 0;
    await expect(
      withRetry(
        async () => {
          calls += 1;
          throw new InvalidRenderRequestError("inválido");
        },
        policy,
        async () => undefined,
      ),
    ).rejects.toBeInstanceOf(InvalidRenderRequestError);
    expect(calls).toBe(1);
  });
});

describe("loadConfig", () => {
  it("usa padrões e lê variáveis de ambiente", () => {
    expect(loadConfig({}).remotion).toMatchObject({ concurrency: 4, browserExecutable: null });
    expect(loadConfig({ RENDER_CONCURRENCY: "6", BROWSER_EXECUTABLE: "/x" }).remotion).toMatchObject({
      concurrency: 6,
      browserExecutable: "/x",
    });
  });

  it("rejeita valores inválidos", () => {
    expect(() => loadConfig({ RENDER_CONCURRENCY: "zero" })).toThrow(/RENDER_CONCURRENCY/);
  });
});
