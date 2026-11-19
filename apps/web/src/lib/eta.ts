/**
 * ETA do render a partir do histórico real (segundos de render por segundo de vídeo),
 * vindo de GET /metrics. Sem histórico, não há estimativa.
 */
export function estimateRemainingSeconds(progress: number, videoSeconds: number, renderSecondsPerVideoSecond: number): number | null {
  if (renderSecondsPerVideoSecond <= 0 || videoSeconds <= 0) {
    return null;
  }
  const remaining = Math.max(0, 1 - Math.min(1, Math.max(0, progress)));
  return Math.ceil(remaining * videoSeconds * renderSecondsPerVideoSecond);
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return s === 0 ? `${m}min` : `${m}min ${s}s`;
}
