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
  validateStoryboard,
} from "../src/index.js";
import { renderSchema, schemaFileName } from "../scripts/generate-json-schema.js";

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

describe("JSON Schema gerado", () => {
  it("está atualizado em relação ao Zod (rode `pnpm generate` se falhar)", () => {
    const committed = readFileSync(join(here, "..", "schemas", schemaFileName("promo")), "utf8");
    expect(committed).toBe(renderSchema("promo"));
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
