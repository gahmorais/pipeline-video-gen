import { z } from "zod";

/** Limite de tamanho de referências a assets (caminho relativo ou URL interna). */
export const MAX_ASSET_REF_CHARS = 2048;

export const HexColorSchema = z
  .string()
  .regex(/^#[0-9a-fA-F]{6}$/, "Cor deve estar no formato #RRGGBB")
  .describe("Cor hexadecimal no formato #RRGGBB");

export const AssetRefSchema = z
  .string()
  .min(1)
  .max(MAX_ASSET_REF_CHARS)
  .describe("Referência a um asset já validado pela API (caminho relativo ou URL interna)");
