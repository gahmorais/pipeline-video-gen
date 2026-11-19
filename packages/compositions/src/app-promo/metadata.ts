import type { CalculateMetadataFunction } from "remotion";
import { totalDurationInFrames, type AppPromoStoryboard } from "@videogen/video-schema";
import { APP_PROMO_CONFIG } from "../shared/video-config";

/** Duração = soma das cenas; dimensões fixas do template (vertical). */
export const calculateAppPromoMetadata: CalculateMetadataFunction<AppPromoStoryboard> = ({ props }) => ({
  durationInFrames: totalDurationInFrames(props.scenes, APP_PROMO_CONFIG.fps),
  fps: APP_PROMO_CONFIG.fps,
  width: APP_PROMO_CONFIG.width,
  height: APP_PROMO_CONFIG.height,
});
