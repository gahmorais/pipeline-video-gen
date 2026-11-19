import { z } from "zod";
import { AssetRefSchema, HexColorSchema } from "./common";

/**
 * Template "promo" — versão 1.
 *
 * Contrato entre LLM, API (Go), Player (web) e render worker.
 * Qualquer mudança incompatível exige nova versão (PromoV2Schema), nunca edição deste schema.
 */

export const PROMO_SCHEMA_VERSION = 1 as const;

export const SCENE_LIMITS = {
  minScenes: 1,
  maxScenes: 12,
  minDurationSec: 1,
  maxTitleDurationSec: 8,
  maxProductDurationSec: 10,
  maxCtaDurationSec: 8,
  maxTitleChars: 60,
  maxProductNameChars: 60,
  maxPriceChars: 20,
  maxCtaChars: 40,
  maxUrlChars: 2048,
} as const;


export const TitleSceneSchema = z
  .object({
    type: z.literal("title"),
    text: z.string().min(1).max(SCENE_LIMITS.maxTitleChars),
    durationSec: z.number().min(SCENE_LIMITS.minDurationSec).max(SCENE_LIMITS.maxTitleDurationSec),
  })
  .strict();

export const ProductSceneSchema = z
  .object({
    type: z.literal("product"),
    name: z.string().min(1).max(SCENE_LIMITS.maxProductNameChars),
    price: z.string().min(1).max(SCENE_LIMITS.maxPriceChars),
    imageUrl: AssetRefSchema,
    durationSec: z.number().min(SCENE_LIMITS.minDurationSec).max(SCENE_LIMITS.maxProductDurationSec),
  })
  .strict();

export const CtaSceneSchema = z
  .object({
    type: z.literal("cta"),
    text: z.string().min(1).max(SCENE_LIMITS.maxCtaChars),
    durationSec: z.number().min(SCENE_LIMITS.minDurationSec).max(SCENE_LIMITS.maxCtaDurationSec),
  })
  .strict();

export const SceneSchema = z.discriminatedUnion("type", [
  TitleSceneSchema,
  ProductSceneSchema,
  CtaSceneSchema,
]);

export const BrandSchema = z
  .object({
    primary: HexColorSchema,
    logoUrl: AssetRefSchema,
  })
  .strict();

export const PromoSchema = z
  .object({
    schemaVersion: z.literal(PROMO_SCHEMA_VERSION),
    brand: BrandSchema,
    scenes: z.array(SceneSchema).min(SCENE_LIMITS.minScenes).max(SCENE_LIMITS.maxScenes),
  })
  .strict();

export type TitleScene = z.infer<typeof TitleSceneSchema>;
export type ProductScene = z.infer<typeof ProductSceneSchema>;
export type CtaScene = z.infer<typeof CtaSceneSchema>;
export type Scene = z.infer<typeof SceneSchema>;
export type Brand = z.infer<typeof BrandSchema>;
export type PromoStoryboard = z.infer<typeof PromoSchema>;
