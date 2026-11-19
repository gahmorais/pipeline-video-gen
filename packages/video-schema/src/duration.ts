import type { Scene } from "./promo.js";

/**
 * Converte segundos em frames. Arredonda para garantir frames inteiros
 * (o Remotion exige durationInFrames inteiro e > 0).
 */
export function secondsToFrames(seconds: number, fps: number): number {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    throw new RangeError(`seconds deve ser > 0, recebido ${seconds}`);
  }
  if (!Number.isInteger(fps) || fps <= 0) {
    throw new RangeError(`fps deve ser inteiro > 0, recebido ${fps}`);
  }
  return Math.max(1, Math.round(seconds * fps));
}

/**
 * Frames de cada cena. A soma por cena (e não o arredondamento do total)
 * garante que o total bata exatamente com a soma das <Series.Sequence>.
 */
export function sceneFrames(scenes: readonly Pick<Scene, "durationSec">[], fps: number): number[] {
  return scenes.map((s) => secondsToFrames(s.durationSec, fps));
}

export function totalDurationInFrames(scenes: readonly Pick<Scene, "durationSec">[], fps: number): number {
  return sceneFrames(scenes, fps).reduce((acc, f) => acc + f, 0);
}

export function totalDurationSec(scenes: readonly Pick<Scene, "durationSec">[]): number {
  return scenes.reduce((acc, s) => acc + s.durationSec, 0);
}
