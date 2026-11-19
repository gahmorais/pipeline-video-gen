import type React from "react";
import { Composition } from "remotion";
import { AppPromoSchema, PromoSchema, templates } from "@videogen/video-schema";
import appPromoFixture from "@videogen/video-schema/fixtures/app-promo/valid/basic.json";
import basicFixture from "@videogen/video-schema/fixtures/valid/promo-basic.json";
import { AppPromo } from "./app-promo/AppPromo";
import { calculateAppPromoMetadata } from "./app-promo/metadata";
import { calculatePromoMetadata } from "./promo/metadata";
import { Promo } from "./promo/Promo";
import { APP_PROMO_CONFIG, VIDEO_CONFIG } from "./shared/video-config";

const promoDefaults = PromoSchema.parse(basicFixture);
const appPromoDefaults = AppPromoSchema.parse(appPromoFixture);

// durationInFrames={1} é provisório: o calculateMetadata deriva a duração das cenas.
export const RemotionRoot: React.FC = () => (
  <>
    <Composition
      id={templates.promo.compositionId}
      component={Promo}
      schema={PromoSchema}
      defaultProps={promoDefaults}
      calculateMetadata={calculatePromoMetadata}
      durationInFrames={1}
      fps={VIDEO_CONFIG.fps}
      width={VIDEO_CONFIG.width}
      height={VIDEO_CONFIG.height}
    />
    <Composition
      id={templates["app-promo"].compositionId}
      component={AppPromo}
      schema={AppPromoSchema}
      defaultProps={appPromoDefaults}
      calculateMetadata={calculateAppPromoMetadata}
      durationInFrames={1}
      fps={APP_PROMO_CONFIG.fps}
      width={APP_PROMO_CONFIG.width}
      height={APP_PROMO_CONFIG.height}
    />
  </>
);
