/**
 * Utilitários visuais puros (sem React), derivados das props.
 */

/** Retorna preto ou branco, o que tiver melhor contraste sobre `hex` (luminância relativa WCAG). */
export function readableTextColor(hex: string): "#000000" | "#FFFFFF" {
  const match = /^#([0-9a-fA-F]{2})([0-9a-fA-F]{2})([0-9a-fA-F]{2})$/.exec(hex);
  if (!match) {
    return "#FFFFFF";
  }
  const [r, g, b] = match.slice(1, 4).map((c) => {
    const v = Number.parseInt(c, 16) / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  const luminance = 0.2126 * r + 0.7152 * g + 0.0722 * b;
  const contrastWithBlack = (luminance + 0.05) / 0.05;
  const contrastWithWhite = 1.05 / (luminance + 0.05);
  return contrastWithBlack >= contrastWithWhite ? "#000000" : "#FFFFFF";
}

export const FONT_STACK =
  'Inter, "Helvetica Neue", Helvetica, Arial, "Segoe UI", Roboto, sans-serif';

/** Frames de fade de entrada/saída de cada cena, limitado a 1/4 da cena. */
export function fadeFrames(sceneDurationInFrames: number, fps: number): number {
  return Math.max(1, Math.min(Math.round(fps * 0.4), Math.floor(sceneDurationInFrames / 4)));
}
