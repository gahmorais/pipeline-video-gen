// API pública do pacote para consumidores (web/Player e render worker).
// Não importa o entry com registerRoot para evitar efeitos colaterais.
export { Promo } from "./promo/Promo";
export { calculatePromoMetadata } from "./promo/metadata";
export { VIDEO_CONFIG } from "./shared/video-config";
export { resolveAssetRef, type AssetResolution } from "./shared/assets";
