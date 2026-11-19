"use client";

import type { ValidationIssue } from "@videogen/video-schema";
import { useEffect, useState } from "react";
import { formatJson, parseJsonDraft } from "@/lib/templates";

interface Props {
  value: unknown;
  issues: ValidationIssue[];
  disabled: boolean;
  onChange: (next: unknown) => void;
}

/**
 * Editor genérico para templates sem formulário dedicado: edita o JSON do roteiro,
 * validado em tempo real pelo mesmo schema do servidor (issues por caminho).
 */
export function JsonStoryboardEditor({ value, issues, disabled, onChange }: Props) {
  const [text, setText] = useState(() => formatJson(value));
  const [syntaxError, setSyntaxError] = useState<string | null>(null);

  // Atualiza o texto quando o valor muda por fora (ex.: "Descartar", snapshot do servidor),
  // sem reformatar enquanto o usuário digita algo equivalente.
  useEffect(() => {
    const current = parseJsonDraft(text);
    if (!current.ok || formatJson(current.value) !== formatJson(value)) {
      setText(formatJson(value));
      setSyntaxError(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value]);

  return (
    <div className="stack">
      <textarea
        className="code"
        spellCheck={false}
        rows={28}
        value={text}
        disabled={disabled}
        aria-label="Roteiro em JSON"
        onChange={(e) => {
          setText(e.target.value);
          const parsed = parseJsonDraft(e.target.value);
          if (parsed.ok) {
            setSyntaxError(null);
            onChange(parsed.value);
          } else {
            setSyntaxError(parsed.error);
          }
        }}
      />
      {syntaxError && <p className="error">JSON inválido: {syntaxError}</p>}
      {!syntaxError && issues.length > 0 && (
        <ul className="issues">
          {issues.map((i) => (
            <li key={`${i.path}:${i.message}`}>
              {i.path && <code>{i.path}</code>} {i.message}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
