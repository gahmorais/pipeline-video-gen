/**
 * Volume da trilha por frame: fade-in no início e fade-out no fim do vídeo.
 * Função pura (sem React) para ser testável.
 */
export function soundtrackVolume(
  frame: number,
  durationInFrames: number,
  fps: number,
  baseVolume: number,
): number {
  const fadeIn = Math.max(1, Math.round(fps * 0.5));
  const fadeOut = Math.max(1, Math.round(fps * 1.5));
  const inFactor = Math.min(1, frame / fadeIn);
  const outFactor = Math.min(1, Math.max(0, (durationInFrames - frame) / fadeOut));
  return Math.max(0, Math.min(1, baseVolume * Math.min(inFactor, outFactor)));
}
