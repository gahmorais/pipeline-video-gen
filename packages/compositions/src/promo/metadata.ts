import type { CalculateMetadataFunction } from "remotion";
import { totalDurationInFrames, type PromoStoryboard } from "@videogen/video-schema";
import { VIDEO_CONFIG } from "../shared/video-config";

/**
 * A duração do vídeo vem exclusivamente da soma de `durationSec` das cenas.
 * O LLM nunca define frames (regra do CLAUDE.md).
 */
export const calculatePromoMetadata: CalculateMetadataFunction<PromoStoryboard> = ({ props }) => ({
  durationInFrames: totalDurationInFrames(props.scenes, VIDEO_CONFIG.fps),
  fps: VIDEO_CONFIG.fps,
  width: VIDEO_CONFIG.width,
  height: VIDEO_CONFIG.height,
});
