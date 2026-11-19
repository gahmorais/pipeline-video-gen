import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { describe, expect, it } from "vitest";
import { ApiClient, ApiError } from "../src/lib/api";
import {
  addScene,
  issuesForScene,
  moveScene,
  parseStoryboard,
  removeScene,
  setBrandColor,
  updateScene,
  validate,
} from "../src/lib/storyboard-editor";

const require = createRequire(import.meta.url);
const fixture = JSON.parse(
  readFileSync(require.resolve("@videogen/video-schema/fixtures/valid/promo-basic.json"), "utf8"),
) as unknown;
const sb = parseStoryboard(fixture)!;

describe("storyboard-editor", () => {
  it("parseStoryboard aceita válido e rejeita inválido", () => {
    expect(sb.scenes).toHaveLength(3);
    expect(parseStoryboard({ schemaVersion: 1 })).toBeNull();
  });

  it("operações são imutáveis", () => {
    const edited = updateScene(sb, 0, { durationSec: 4 });
    expect(edited.scenes[0]?.durationSec).toBe(4);
    expect(sb.scenes[0]?.durationSec).toBe(2);
  });

  it("move, remove e respeita limites", () => {
    expect(moveScene(sb, 0, 1).scenes.map((s) => s.type)).toEqual(["product", "title", "cta"]);
    expect(moveScene(sb, 0, -1)).toBe(sb);
    expect(removeScene(sb, 1).scenes.map((s) => s.type)).toEqual(["title", "cta"]);
  });

  it("addScene insere antes do CTA final", () => {
    expect(addScene(sb, "product").scenes.map((s) => s.type)).toEqual(["title", "product", "product", "cta"]);
    expect(addScene(sb, "cta").scenes.map((s) => s.type)).toEqual(["title", "product", "cta", "cta"]);
  });

  it("addScene não passa do máximo de cenas", () => {
    let full = sb;
    for (let i = 0; i < 20; i++) full = addScene(full, "title");
    expect(full.scenes).toHaveLength(12);
  });

  it("validate devolve issues por caminho e issuesForScene filtra", () => {
    const invalid = setBrandColor(updateScene(sb, 1, { durationSec: 99 }), "azul");
    const issues = validate(invalid);
    expect(issues.map((i) => i.path).sort()).toEqual(["brand.primary", "scenes.1.durationSec"]);
    expect(issuesForScene(issues, 1)).toHaveLength(1);
    expect(issuesForScene(issues, 0)).toHaveLength(0);
    expect(validate(sb)).toEqual([]);
  });
});

describe("ApiClient", () => {
  it("envia idempotency key e converte problem+json em ApiError", async () => {
    const calls: Array<{ url: string; init: RequestInit }> = [];
    const client = new ApiClient("http://api", async (url, init) => {
      calls.push({ url: String(url), init: init ?? {} });
      return new Response(
        JSON.stringify({ type: "validation-error", title: "x", issues: [{ path: "scenes", message: "vazio" }] }),
        { status: 422 },
      );
    });
    const err = await client.requestRender("job 1", "key-1").catch((e: unknown) => e);
    expect(calls[0]?.url).toBe("http://api/videos/job%201/render");
    expect((calls[0]?.init.headers as Record<string, string>)["Idempotency-Key"]).toBe("key-1");
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).issues).toEqual([{ path: "scenes", message: "vazio" }]);
  });

  it("resolve mediaUrl relativo contra a base", () => {
    expect(new ApiClient("http://localhost:8080").absoluteUrl("/videos/a/media?r=b")).toBe(
      "http://localhost:8080/videos/a/media?r=b",
    );
  });
});
