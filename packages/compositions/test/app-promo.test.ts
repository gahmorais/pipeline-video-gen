import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { describe, expect, it } from "vitest";
import { AppPromoSchema, templateIds } from "@videogen/video-schema";
import { soundtrackVolume } from "../src/app-promo/audio";
import { calculateAppPromoMetadata } from "../src/app-promo/metadata";
import { compositions } from "../src/registry";
import { APP_PROMO_CONFIG } from "../src/shared/video-config";

const require = createRequire(import.meta.url);
const fixture = AppPromoSchema.parse(
  JSON.parse(readFileSync(require.resolve("@videogen/video-schema/fixtures/app-promo/valid/basic.json"), "utf8")),
);

describe("soundtrackVolume", () => {
  const fps = 30;
  const total = 300;

  it("começa em silêncio e sobe no fade-in", () => {
    expect(soundtrackVolume(0, total, fps, 0.8)).toBe(0);
    expect(soundtrackVolume(7, total, fps, 0.8)).toBeCloseTo(0.8 * (7 / 15));
    expect(soundtrackVolume(15, total, fps, 0.8)).toBeCloseTo(0.8);
  });

  it("mantém o volume base no meio e faz fade-out até zero", () => {
    expect(soundtrackVolume(150, total, fps, 0.5)).toBe(0.5);
    expect(soundtrackVolume(total - 45, total, fps, 0.5)).toBeCloseTo(0.5);
    expect(soundtrackVolume(total, total, fps, 0.5)).toBe(0);
  });

  it("nunca sai de [0, 1]", () => {
    for (let f = -10; f <= total + 10; f += 5) {
      const v = soundtrackVolume(f, total, fps, 1);
      expect(v).toBeGreaterThanOrEqual(0);
      expect(v).toBeLessThanOrEqual(1);
    }
  });
});

describe("calculateAppPromoMetadata", () => {
  it("é vertical 1080x1920 e soma as cenas", async () => {
    const meta = await calculateAppPromoMetadata({
      props: fixture,
      defaultProps: fixture,
      abortSignal: new AbortController().signal,
      compositionId: "AppPromo",
      isRendering: false,
    });
    // 2 + 3 + 2 + 2 = 9 s a 30 fps
    expect(meta).toEqual({ durationInFrames: 270, fps: 30, width: 1080, height: 1920 });
  });
});

describe("registro de composições", () => {
  it("cobre todos os templates do schema", () => {
    expect(Object.keys(compositions).sort()).toEqual([...templateIds].sort());
    expect(compositions["app-promo"].config).toEqual(APP_PROMO_CONFIG);
    expect(compositions.promo.config.width).toBe(1920);
  });
});
