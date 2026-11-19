import type React from "react";
import type { StoryboardOf, TemplateId } from "@videogen/video-schema";
import { AppPromo } from "./app-promo/AppPromo";
import { Promo } from "./promo/Promo";
import { APP_PROMO_CONFIG, VIDEO_CONFIG } from "./shared/video-config";

export interface CompositionConfig {
  fps: number;
  width: number;
  height: number;
}

export interface CompositionEntry<T extends TemplateId> {
  component: React.FC<StoryboardOf<T>>;
  config: CompositionConfig;
}

/**
 * Componente e dimensões de cada template. Compartilhado pelo Root (Studio/render)
 * e pelo Player no frontend: o preview usa exatamente a composição do render.
 */
export const compositions: { [T in TemplateId]: CompositionEntry<T> } = {
  promo: { component: Promo, config: VIDEO_CONFIG },
  "app-promo": { component: AppPromo, config: APP_PROMO_CONFIG },
};
