/**
 * Operações genéricas por template (parse/validação), usando o mesmo schema
 * Zod do servidor e do render worker.
 */
import {
  isTemplateId,
  templates,
  validateStoryboard,
  type StoryboardOf,
  type TemplateId,
  type ValidationIssue,
} from "@videogen/video-schema";

export type AnyStoryboard = StoryboardOf<TemplateId>;

export interface TemplateOption {
  id: TemplateId;
  label: string;
  description: string;
}

export const TEMPLATE_OPTIONS: TemplateOption[] = [
  { id: "promo", label: "Promoção de produtos", description: "Horizontal 16:9 — ofertas de varejo" },
  { id: "app-promo", label: "Divulgação de app", description: "Vertical 9:16 — Reels, TikTok e Shorts" },
];

export function resolveTemplateId(value: string): TemplateId | null {
  return isTemplateId(value) ? value : null;
}

export function parseFor<T extends TemplateId>(templateId: T, raw: unknown): StoryboardOf<T> | null {
  const result = validateStoryboard(templateId, raw);
  return result.success ? result.data : null;
}

export function validateFor(templateId: TemplateId, draft: unknown): ValidationIssue[] {
  const result = validateStoryboard(templateId, draft);
  return result.success ? [] : result.issues;
}

export function templateLabel(templateId: TemplateId): string {
  return TEMPLATE_OPTIONS.find((t) => t.id === templateId)?.label ?? templates[templateId].id;
}

/** Converte o texto do editor JSON em objeto, sem lançar exceção. */
export function parseJsonDraft(text: string): { ok: true; value: unknown } | { ok: false; error: string } {
  try {
    return { ok: true, value: JSON.parse(text) as unknown };
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : "JSON inválido" };
  }
}

export function formatJson(value: unknown): string {
  return JSON.stringify(value, null, 2);
}
