import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { describe, expect, it } from "vitest";
import { formatJson, parseFor, parseJsonDraft, resolveTemplateId, TEMPLATE_OPTIONS, validateFor } from "../src/lib/templates";

const require = createRequire(import.meta.url);
const read = (p: string): unknown => JSON.parse(readFileSync(require.resolve(`@videogen/video-schema/${p}`), "utf8"));
const app = read("fixtures/app-promo/valid/basic.json");
const promo = read("fixtures/valid/promo-basic.json");

describe("templates", () => {
  it("oferece os dois formatos na criação", () => {
    expect(TEMPLATE_OPTIONS.map((t) => t.id)).toEqual(["promo", "app-promo"]);
  });

  it("resolve ids conhecidos e rejeita desconhecidos", () => {
    expect(resolveTemplateId("app-promo")).toBe("app-promo");
    expect(resolveTemplateId("nope")).toBeNull();
  });

  it("parseFor e validateFor usam o schema do template", () => {
    expect(parseFor("app-promo", app)).not.toBeNull();
    expect(parseFor("app-promo", promo)).toBeNull();
    expect(validateFor("app-promo", app)).toEqual([]);
    const issues = validateFor("app-promo", { ...(app as object), scenes: [] });
    expect(issues.map((i) => i.path)).toContain("scenes");
  });

  it("parseJsonDraft não lança exceção e formatJson é estável", () => {
    expect(parseJsonDraft('{"a":1}')).toEqual({ ok: true, value: { a: 1 } });
    expect(parseJsonDraft("{").ok).toBe(false);
    expect(formatJson({ a: [1] })).toBe('{\n  "a": [\n    1\n  ]\n}');
  });
});
