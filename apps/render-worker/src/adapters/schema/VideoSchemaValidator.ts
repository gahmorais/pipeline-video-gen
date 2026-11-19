import { isTemplateId, templates, validateStoryboard } from "@videogen/video-schema";
import type { StoryboardValidation, StoryboardValidator } from "../../ports";

/** Adapter que valida o storyboard usando o pacote compartilhado @videogen/video-schema. */
export class VideoSchemaValidator implements StoryboardValidator {
  validate(templateId: string, templateVersion: number, storyboard: unknown): StoryboardValidation {
    if (!isTemplateId(templateId)) {
      return { valid: false, reason: `template desconhecido: ${templateId}`, issues: [] };
    }
    const template = templates[templateId];
    if (template.version !== templateVersion) {
      return {
        valid: false,
        reason: `versão de template não suportada: ${templateId}@${templateVersion} (worker suporta @${template.version})`,
        issues: [],
      };
    }
    const result = validateStoryboard(templateId, storyboard);
    if (!result.success) {
      return { valid: false, reason: "storyboard inválido", issues: result.issues };
    }
    return {
      valid: true,
      compositionId: template.compositionId,
      inputProps: result.data as unknown as Record<string, unknown>,
    };
  }
}
