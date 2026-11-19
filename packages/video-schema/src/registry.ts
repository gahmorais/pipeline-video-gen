import { z } from "zod";
import { PROMO_SCHEMA_VERSION, PromoSchema } from "./promo.js";

/**
 * Registro de templates. A chave (templateId@version) é gravada em cada job
 * para garantir re-render reprodutível.
 */
export const templates = {
  promo: {
    id: "promo",
    version: PROMO_SCHEMA_VERSION,
    compositionId: "Promo",
    schema: PromoSchema,
  },
} as const;

export type TemplateId = keyof typeof templates;

export function isTemplateId(value: string): value is TemplateId {
  return Object.hasOwn(templates, value);
}

export type ValidationResult<T> =
  | { success: true; data: T }
  | { success: false; issues: ValidationIssue[] };

export interface ValidationIssue {
  path: string;
  message: string;
}

export function validateStoryboard(
  templateId: TemplateId,
  input: unknown,
): ValidationResult<z.infer<(typeof templates)[TemplateId]["schema"]>> {
  const result = templates[templateId].schema.safeParse(input);
  if (result.success) {
    return { success: true, data: result.data };
  }
  return {
    success: false,
    issues: result.error.issues.map((i) => ({
      path: i.path.map(String).join("."),
      message: i.message,
    })),
  };
}

/** JSON Schema (draft 2020-12) do template — fonte para Go e para o structured output do LLM. */
export function toJsonSchema(templateId: TemplateId): Record<string, unknown> {
  const t = templates[templateId];
  const schema = z.toJSONSchema(t.schema, { target: "draft-2020-12" }) as Record<string, unknown>;
  return {
    $id: `https://videogen.local/schemas/${t.id}.v${t.version}.schema.json`,
    title: `${t.id} storyboard v${t.version}`,
    ...schema,
  };
}
