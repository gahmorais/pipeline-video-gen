/**
 * Configuração de vídeo padrão. Fixa por decisão de projeto (MacBook Air M2):
 * 1080p @ 30 fps. Mudanças aqui alteram o resultado de renders antigos,
 * portanto exigem nova versão de template.
 */
export const VIDEO_CONFIG = {
  fps: 30,
  width: 1920,
  height: 1080,
} as const;
