import { z } from "zod";
import { AssetRefSchema, HexColorSchema } from "./common";

/**
 * Template "app-promo" — versão 1.
 *
 * Divulgação de apps/jogos em formato vertical (Reels, TikTok, Shorts):
 * trechos de gameplay (clip), telas com destaque (feature), chamada para as lojas
 * e trilha sonora opcional. Mudança incompatível = nova versão, nunca edição desta.
 */

export const APP_PROMO_SCHEMA_VERSION = 1 as const;

export const APP_PROMO_LIMITS = {
  minScenes: 1,
  maxScenes: 15,
  minDurationSec: 1,
  maxTitleDurationSec: 6,
  maxClipDurationSec: 15,
  maxFeatureDurationSec: 8,
  maxCtaDurationSec: 8,
  maxClipStartSec: 600,
  maxAppNameChars: 30,
  maxTitleChars: 50,
  maxCaptionChars: 40,
  maxFeatureNameChars: 30,
  maxTaglineChars: 40,
  maxCtaChars: 30,
} as const;

const L = APP_PROMO_LIMITS;

const duration = (max: number) => z.number().min(L.minDurationSec).max(max);

export const StoreSchema = z.enum(["google-play", "app-store"]);

export const AppTitleSceneSchema = z
  .object({
    type: z.literal("title"),
    text: z.string().min(1).max(L.maxTitleChars),
    durationSec: duration(L.maxTitleDurationSec),
  })
  .strict();

export const AppClipSceneSchema = z
  .object({
    type: z.literal("clip"),
    videoUrl: AssetRefSchema.describe("Trecho de gameplay/gravação de tela (mp4), já enviado à API"),
    startSec: z.number().min(0).max(L.maxClipStartSec).describe("Ponto de início dentro do vídeo de origem"),
    caption: z.string().min(1).max(L.maxCaptionChars).optional(),
    durationSec: duration(L.maxClipDurationSec),
  })
  .strict();

export const AppFeatureSceneSchema = z
  .object({
    type: z.literal("feature"),
    name: z.string().min(1).max(L.maxFeatureNameChars),
    tagline: z.string().min(1).max(L.maxTaglineChars),
    imageUrl: AssetRefSchema.describe("Screenshot da tela em destaque"),
    durationSec: duration(L.maxFeatureDurationSec),
  })
  .strict();

export const AppCtaSceneSchema = z
  .object({
    type: z.literal("cta"),
    text: z.string().min(1).max(L.maxCtaChars),
    stores: z.array(StoreSchema).min(1).max(2),
    durationSec: duration(L.maxCtaDurationSec),
  })
  .strict();

export const AppSceneSchema = z.discriminatedUnion("type", [
  AppTitleSceneSchema,
  AppClipSceneSchema,
  AppFeatureSceneSchema,
  AppCtaSceneSchema,
]);

export const AppBrandSchema = z
  .object({
    appName: z.string().min(1).max(L.maxAppNameChars),
    primary: HexColorSchema,
    logoUrl: AssetRefSchema,
  })
  .strict();

export const SoundtrackSchema = z
  .object({
    src: AssetRefSchema.describe("Trilha sonora (mp3/m4a), já enviada à API"),
    volume: z.number().min(0).max(1),
  })
  .strict();

export const AppPromoSchema = z
  .object({
    schemaVersion: z.literal(APP_PROMO_SCHEMA_VERSION),
    brand: AppBrandSchema,
    soundtrack: SoundtrackSchema.optional(),
    scenes: z.array(AppSceneSchema).min(L.minScenes).max(L.maxScenes),
  })
  .strict();

export type Store = z.infer<typeof StoreSchema>;
export type AppTitleScene = z.infer<typeof AppTitleSceneSchema>;
export type AppClipScene = z.infer<typeof AppClipSceneSchema>;
export type AppFeatureScene = z.infer<typeof AppFeatureSceneSchema>;
export type AppCtaScene = z.infer<typeof AppCtaSceneSchema>;
export type AppScene = z.infer<typeof AppSceneSchema>;
export type AppBrand = z.infer<typeof AppBrandSchema>;
export type Soundtrack = z.infer<typeof SoundtrackSchema>;
export type AppPromoStoryboard = z.infer<typeof AppPromoSchema>;
