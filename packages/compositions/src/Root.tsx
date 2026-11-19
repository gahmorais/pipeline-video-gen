import type React from "react";
import { Composition } from "remotion";
import { PromoSchema, templates, type PromoStoryboard } from "@videogen/video-schema";
import basicFixture from "@videogen/video-schema/fixtures/valid/promo-basic.json";
import { calculatePromoMetadata } from "./promo/metadata";
import { Promo } from "./promo/Promo";
import { VIDEO_CONFIG } from "./shared/video-config";

const defaultProps = PromoSchema.parse(basicFixture) satisfies PromoStoryboard;

export const RemotionRoot: React.FC = () => (
  <Composition
    id={templates.promo.compositionId}
    component={Promo}
    schema={PromoSchema}
    defaultProps={defaultProps}
    calculateMetadata={calculatePromoMetadata}
    // Valor provisório: o calculateMetadata sobrescreve a partir das cenas.
    durationInFrames={1}
    fps={VIDEO_CONFIG.fps}
    width={VIDEO_CONFIG.width}
    height={VIDEO_CONFIG.height}
  />
);
