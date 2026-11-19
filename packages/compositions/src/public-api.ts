// API pública do pacote para consumidores (web/Player e render worker).
// Não importa o entry com registerRoot para evitar efeitos colaterais.
export { Promo } from "./promo/Promo";
export { calculatePromoMetadata } from "./promo/metadata";
export { AppPromo } from "./app-promo/AppPromo";
export { calculateAppPromoMetadata } from "./app-promo/metadata";
export { compositions, type CompositionConfig, type CompositionEntry } from "./registry";
export { APP_PROMO_CONFIG, VERTICAL_SAFE_AREA, VIDEO_CONFIG } from "./shared/video-config";
export { resolveAssetRef, type AssetResolution } from "./shared/assets";
