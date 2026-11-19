import { describe, expect, it } from "vitest";
import { estimateRemainingSeconds, formatDuration } from "../src/lib/eta";

describe("ETA", () => {
  it("estima o restante pelo histórico", () => {
    // 10 s de vídeo, 3 s de render por s de vídeo, 40% feito → 18 s
    expect(estimateRemainingSeconds(0.4, 10, 3)).toBe(18);
    expect(estimateRemainingSeconds(1, 10, 3)).toBe(0);
    expect(estimateRemainingSeconds(1.5, 10, 3)).toBe(0);
  });

  it("sem histórico não estima", () => {
    expect(estimateRemainingSeconds(0.5, 10, 0)).toBeNull();
    expect(estimateRemainingSeconds(0.5, 0, 3)).toBeNull();
  });

  it("formata durações", () => {
    expect(formatDuration(45)).toBe("45s");
    expect(formatDuration(120)).toBe("2min");
    expect(formatDuration(135)).toBe("2min 15s");
  });
});
