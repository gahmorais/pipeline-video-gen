/**
 * Emite progresso apenas quando cruza um novo degrau (ex.: a cada 5%),
 * evitando inundar a API/SSE com centenas de eventos por render.
 * Garante monotonicidade (progresso nunca regride).
 */
export function createProgressThrottle(
  emit: (progress: number) => void,
  stepPercent = 5,
): (progress: number) => void {
  let lastStep = -1;
  return (progress: number) => {
    const clamped = Math.min(1, Math.max(0, progress));
    const step = Math.floor((clamped * 100) / stepPercent);
    if (step > lastStep) {
      lastStep = step;
      emit(Math.min(1, (step * stepPercent) / 100));
    }
  };
}
