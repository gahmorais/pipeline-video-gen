import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  isTemplateId,
  totalDurationInFrames,
  totalDurationSec,
  sceneFrames,
  secondsToFrames,
  templateIds,
  templates,
  validateStoryboard,
} from "../src/index";
import { renderSchema, schemaFileName } from "../scripts/generate-json-schema";

const here = dirname(fileURLToPath(import.meta.url));
const fixtures = join(here, "..", "fixtures");

interface InvalidCase {
  name: string;
  errorPath: string;
  input: unknown;
}

const readJson = (path: string): unknown => JSON.parse(readFileSync(path, "utf8"));

describe("PromoSchema — fixtures válidas", () => {
  const files = readdirSync(join(fixtures, "valid")).filter((f) => f.endsWith(".json"));

  it("possui fixtures", () => {
    expect(files.length).toBeGreaterThan(0);
  });

  it.each(files)("%s é válido", (file) => {
    const result = validateStoryboard("promo", readJson(join(fixtures, "valid", file)));
    expect(result).toMatchObject({ success: true });
  });
});

describe("PromoSchema — fixtures inválidas", () => {
  const cases = readJson(join(fixtures, "invalid", "cases.json")) as InvalidCase[];

  it.each(cases.map((c) => [c.name, c] as const))("rejeita: %s", (_name, c) => {
    const result = validateStoryboard("promo", c.input);
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.issues.map((i) => i.path)).toContain(c.errorPath);
    }
  });
});

describe("duração", () => {
  it("converte segundos em frames arredondando", () => {
    expect(secondsToFrames(1.5, 30)).toBe(45);
    expect(secondsToFrames(1.01, 30)).toBe(30);
  });

  it("rejeita entradas inválidas", () => {
    expect(() => secondsToFrames(0, 30)).toThrow(RangeError);
    expect(() => secondsToFrames(1, 29.97)).toThrow(RangeError);
  });

  it("total em frames é a soma dos frames por cena", () => {
    const scenes = [{ durationSec: 2 }, { durationSec: 3 }, { durationSec: 1.5 }];
    expect(sceneFrames(scenes, 30)).toEqual([60, 90, 45]);
    expect(totalDurationInFrames(scenes, 30)).toBe(195);
    expect(totalDurationSec(scenes)).toBe(6.5);
  });
});

describe("registro de templates", () => {
  it("identifica templates conhecidos", () => {
    expect(isTemplateId("promo")).toBe(true);
    expect(isTemplateId("toString")).toBe(false);
    expect(isTemplateId("unknown")).toBe(false);
  });
});

describe("app-promo", () => {
  const dir = join(fixtures, "app-promo");
  const valid = readdirSync(join(dir, "valid")).filter((f) => f.endsWith(".json"));
  const cases = readJson(join(dir, "invalid", "cases.json")) as InvalidCase[];

  it.each(valid)("aceita %s", (file) => {
    expect(validateStoryboard("app-promo", readJson(join(dir, "valid", file)))).toMatchObject({ success: true });
  });

  it.each(cases.map((c) => [c.name, c] as const))("rejeita: %s", (_name, c) => {
    const result = validateStoryboard("app-promo", c.input);
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.issues.map((i) => i.path)).toContain(c.errorPath);
    }
  });

  it("templates não aceitam storyboards um do outro", () => {
    const promo = readJson(join(fixtures, "valid", "promo-basic.json"));
    const app = readJson(join(dir, "valid", "basic.json"));
    expect(validateStoryboard("app-promo", promo).success).toBe(false);
    expect(validateStoryboard("promo", app).success).toBe(false);
  });

  it("registro expõe os dois templates", () => {
    expect(templateIds.sort()).toEqual(["app-promo", "promo"]);
    expect(templates["app-promo"].compositionId).toBe("AppPromo");
  });
});

describe("JSON Schema gerado", () => {
  it.each(templateIds)("%s está atualizado em relação ao Zod (rode `pnpm generate` se falhar)", (id) => {
    const committed = readFileSync(join(here, "..", "schemas", schemaFileName(id)), "utf8");
    expect(committed).toBe(renderSchema(id));
  });

  it("é determinístico", () => {
    expect(renderSchema("promo")).toBe(renderSchema("promo"));
  });

  it("proíbe propriedades extras e usa draft 2020-12", () => {
    const schema = JSON.parse(renderSchema("promo")) as Record<string, unknown>;
    expect(schema.$schema).toBe("https://json-schema.org/draft/2020-12/schema");
    expect(schema.additionalProperties).toBe(false);
  });
});

describe("exemplos em docs/examples", () => {
  // Convenção de nome: <nome>.<templateId>.json
  const examplesDir = join(here, "..", "..", "..", "docs", "examples");
  const files = readdirSync(examplesDir).filter((f) => f.endsWith(".json"));

  it("existem exemplos", () => {
    expect(files.length).toBeGreaterThan(0);
  });

  it.each(files)("%s é válido para o template do nome do arquivo", (file) => {
    const templateId = file.split(".").at(-2) ?? "";
    expect(isTemplateId(templateId)).toBe(true);
    if (isTemplateId(templateId)) {
      const result = validateStoryboard(templateId, readJson(join(examplesDir, file)));
      expect(result).toMatchObject({ success: true });
    }
  });
});
