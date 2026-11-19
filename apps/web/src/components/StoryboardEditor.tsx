"use client";

import { SCENE_LIMITS, type PromoStoryboard, type Scene, type ValidationIssue } from "@videogen/video-schema";
import {
  addScene,
  issuesForScene,
  moveScene,
  removeScene,
  setBrandColor,
  updateScene,
  type SceneType,
} from "@/lib/storyboard-editor";

interface Props {
  draft: PromoStoryboard;
  issues: ValidationIssue[];
  disabled: boolean;
  onChange: (next: PromoStoryboard) => void;
}

const SCENE_LABEL: Record<SceneType, string> = { title: "Título", product: "Produto", cta: "Chamada" };

/**
 * Editor controlado: recebe o rascunho e devolve um novo a cada alteração.
 * Toda a lógica de mutação está em lib/storyboard-editor (funções puras).
 * Textos são renderizados pelo React (escapados) — nunca dangerouslySetInnerHTML.
 */
export function StoryboardEditor({ draft, issues, disabled, onChange }: Props) {
  const brandIssues = issues.filter((i) => i.path.startsWith("brand"));
  const listIssues = issues.filter((i) => i.path === "scenes" || i.path === "");

  return (
    <fieldset className="stack" disabled={disabled}>
      <div className="row">
        <label className="label" htmlFor="brand-color">
          Cor da marca
        </label>
        <input
          id="brand-color"
          type="color"
          value={/^#[0-9a-fA-F]{6}$/.test(draft.brand.primary) ? draft.brand.primary : "#0B5FFF"}
          onChange={(e) => onChange(setBrandColor(draft, e.target.value.toUpperCase()))}
        />
        <code>{draft.brand.primary}</code>
      </div>
      <Issues issues={[...brandIssues, ...listIssues]} />

      <ol className="scenes">
        {draft.scenes.map((scene, index) => (
          // Cenas não têm id estável; o índice é a identidade dentro do rascunho.
          <li key={index} className="scene card">
            <div className="row between">
              <strong>
                {index + 1}. {SCENE_LABEL[scene.type]}
              </strong>
              <div className="row">
                <button type="button" className="ghost" aria-label="Mover para cima" onClick={() => onChange(moveScene(draft, index, -1))}>
                  ↑
                </button>
                <button type="button" className="ghost" aria-label="Mover para baixo" onClick={() => onChange(moveScene(draft, index, 1))}>
                  ↓
                </button>
                <button
                  type="button"
                  className="ghost danger"
                  disabled={draft.scenes.length <= SCENE_LIMITS.minScenes}
                  onClick={() => onChange(removeScene(draft, index))}
                >
                  Remover
                </button>
              </div>
            </div>
            <SceneFields scene={scene} onChange={(patch) => onChange(updateScene(draft, index, patch))} />
            <Issues issues={issuesForScene(issues, index)} />
          </li>
        ))}
      </ol>

      <div className="row">
        <span className="muted">Adicionar:</span>
        {(Object.keys(SCENE_LABEL) as SceneType[]).map((type) => (
          <button
            key={type}
            type="button"
            className="ghost"
            disabled={draft.scenes.length >= SCENE_LIMITS.maxScenes}
            onClick={() => onChange(addScene(draft, type))}
          >
            + {SCENE_LABEL[type]}
          </button>
        ))}
      </div>
    </fieldset>
  );
}

function SceneFields({ scene, onChange }: { scene: Scene; onChange: (patch: Partial<Scene>) => void }) {
  const duration = (
    <label className="field">
      Duração (s)
      <input
        type="number"
        step={0.5}
        min={SCENE_LIMITS.minDurationSec}
        value={scene.durationSec}
        onChange={(e) => onChange({ durationSec: Number(e.target.value) })}
      />
    </label>
  );

  switch (scene.type) {
    case "title":
    case "cta":
      return (
        <div className="fields">
          <label className="field grow">
            Texto
            <input
              value={scene.text}
              maxLength={scene.type === "title" ? SCENE_LIMITS.maxTitleChars : SCENE_LIMITS.maxCtaChars}
              onChange={(e) => onChange({ text: e.target.value })}
            />
          </label>
          {duration}
        </div>
      );
    case "product":
      return (
        <div className="fields">
          <label className="field grow">
            Nome
            <input value={scene.name} maxLength={SCENE_LIMITS.maxProductNameChars} onChange={(e) => onChange({ name: e.target.value })} />
          </label>
          <label className="field">
            Preço
            <input value={scene.price} maxLength={SCENE_LIMITS.maxPriceChars} onChange={(e) => onChange({ price: e.target.value })} />
          </label>
          {duration}
        </div>
      );
  }
}

function Issues({ issues }: { issues: ValidationIssue[] }) {
  if (issues.length === 0) return null;
  return (
    <ul className="issues">
      {issues.map((i) => (
        <li key={`${i.path}:${i.message}`}>
          {i.path && <code>{i.path}</code>} {i.message}
        </li>
      ))}
    </ul>
  );
}
