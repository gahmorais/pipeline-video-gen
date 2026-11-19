/**
 * Operações puras (imutáveis) sobre o storyboard, usadas pelo editor.
 * Sem React: testáveis isoladamente.
 */
import {
  PromoSchema,
  SCENE_LIMITS,
  validateStoryboard,
  type PromoStoryboard,
  type Scene,
  type ValidationIssue,
} from "@videogen/video-schema";

export type SceneType = Scene["type"];

export function parseStoryboard(raw: unknown): PromoStoryboard | null {
  const result = PromoSchema.safeParse(raw);
  return result.success ? result.data : null;
}

export function validate(sb: PromoStoryboard): ValidationIssue[] {
  const result = validateStoryboard("promo", sb);
  return result.success ? [] : result.issues;
}

export function updateScene(sb: PromoStoryboard, index: number, patch: Partial<Scene>): PromoStoryboard {
  return {
    ...sb,
    scenes: sb.scenes.map((scene, i) => (i === index ? ({ ...scene, ...patch } as Scene) : scene)),
  };
}

export function removeScene(sb: PromoStoryboard, index: number): PromoStoryboard {
  return { ...sb, scenes: sb.scenes.filter((_, i) => i !== index) };
}

export function moveScene(sb: PromoStoryboard, index: number, delta: -1 | 1): PromoStoryboard {
  const target = index + delta;
  if (target < 0 || target >= sb.scenes.length) {
    return sb;
  }
  const scenes = [...sb.scenes];
  const [moved] = scenes.splice(index, 1);
  scenes.splice(target, 0, moved as Scene);
  return { ...sb, scenes };
}

export function newScene(type: SceneType): Scene {
  switch (type) {
    case "title":
      return { type, text: "Novo título", durationSec: 2 };
    case "product":
      return { type, name: "Produto", price: "Consulte", imageUrl: "assets/product-placeholder.png", durationSec: 3 };
    case "cta":
      return { type, text: "Compre agora", durationSec: 2 };
  }
}

/** Insere antes da cena CTA final (quando existir), mantendo o fechamento do vídeo. */
export function addScene(sb: PromoStoryboard, type: SceneType): PromoStoryboard {
  if (sb.scenes.length >= SCENE_LIMITS.maxScenes) {
    return sb;
  }
  const scenes = [...sb.scenes];
  const last = scenes.at(-1);
  const insertAt = type !== "cta" && last?.type === "cta" ? scenes.length - 1 : scenes.length;
  scenes.splice(insertAt, 0, newScene(type));
  return { ...sb, scenes };
}

export function setBrandColor(sb: PromoStoryboard, color: string): PromoStoryboard {
  return { ...sb, brand: { ...sb.brand, primary: color } };
}

/** Issues de uma cena específica (prefixo "scenes.<i>"). */
export function issuesForScene(issues: ValidationIssue[], index: number): ValidationIssue[] {
  const prefix = `scenes.${index}`;
  return issues.filter((i) => i.path === prefix || i.path.startsWith(`${prefix}.`));
}

export function sameStoryboard(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}
