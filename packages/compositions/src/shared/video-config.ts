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

/**
 * Template app-promo: vertical 9:16 (Reels, TikTok, Shorts), 30 fps.
 */
export const APP_PROMO_CONFIG = {
  fps: 30,
  width: 1080,
  height: 1920,
} as const;

/**
 * Área segura do vertical: as redes sobrepõem UI no topo (perfil/legenda)
 * e na base (botões, descrição). Conteúdo essencial fica entre essas faixas.
 */
export const VERTICAL_SAFE_AREA = {
  top: 220,
  bottom: 420,
  side: 72,
} as const;
