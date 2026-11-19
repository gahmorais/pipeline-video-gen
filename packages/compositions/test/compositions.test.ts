import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { describe, expect, it } from "vitest";
import { PromoSchema } from "@videogen/video-schema";
import { resolveAssetRef } from "../src/shared/assets";
import { fadeFrames, readableTextColor } from "../src/promo/theme";
import { calculatePromoMetadata } from "../src/promo/metadata";
import { VIDEO_CONFIG } from "../src/shared/video-config";

const require = createRequire(import.meta.url);
const fixturePath = require.resolve("@videogen/video-schema/fixtures/valid/promo-basic.json");
const basic = PromoSchema.parse(JSON.parse(readFileSync(fixturePath, "utf8")));

const toStatic = (p: string) => `/public/${p}`;

describe("resolveAssetRef", () => {
  it.each([
    ["assets/logo.png", { kind: "static", src: "/public/assets/logo.png" }],
    ["/assets/logo.png", { kind: "static", src: "/public/assets/logo.png" }],
    ["https://cdn.local/a.png", { kind: "remote", src: "https://cdn.local/a.png" }],
  ])("aceita %s", (ref, expected) => {
    expect(resolveAssetRef(ref, toStatic)).toEqual(expected);
  });

  it.each([
    "",
    "   ",
    "javascript:alert(1)",
    "data:image/png;base64,AAAA",
    "file:///etc/passwd",
    "../secrets.png",
    "assets/../../x.png",
    "assets/<script>.png",
  ])("rejeita %j", (ref) => {
    expect(resolveAssetRef(ref, toStatic).kind).toBe("invalid");
  });
});

describe("readableTextColor", () => {
  it("usa texto branco sobre cores escuras e preto sobre claras", () => {
    expect(readableTextColor("#000000")).toBe("#FFFFFF");
    expect(readableTextColor("#0B5FFF")).toBe("#FFFFFF");
    expect(readableTextColor("#FFFFFF")).toBe("#000000");
    expect(readableTextColor("#FFD400")).toBe("#000000");
  });

  it("tem fallback para entrada inválida", () => {
    expect(readableTextColor("azul")).toBe("#FFFFFF");
  });
});

describe("fadeFrames", () => {
  it("limita o fade a 1/4 da cena", () => {
    expect(fadeFrames(30, 30)).toBe(7);
    expect(fadeFrames(300, 30)).toBe(12);
    expect(fadeFrames(2, 30)).toBe(1);
  });
});

describe("calculatePromoMetadata", () => {
  it("deriva duração e dimensões das cenas", async () => {
    const metadata = await calculatePromoMetadata({
      props: basic,
      defaultProps: basic,
      abortSignal: new AbortController().signal,
      compositionId: "Promo",
      isRendering: false,
    });
    // 2s + 3s + 1.5s a 30 fps
    expect(metadata).toEqual({
      durationInFrames: 195,
      fps: VIDEO_CONFIG.fps,
      width: VIDEO_CONFIG.width,
      height: VIDEO_CONFIG.height,
    });
  });
});
