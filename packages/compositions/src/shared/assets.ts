/**
 * Resolução de referências de assets vindas do storyboard.
 *
 * Função pura e sem dependência do Remotion para ser testável em Node.
 * O chamador injeta `toStatic` (normalmente `staticFile` do Remotion).
 *
 * Regras de segurança (defesa em profundidade — a API já valida os assets):
 * - caminhos relativos simples são servidos da pasta `public/` do bundle;
 * - URLs https/http são aceitas (a API decide quais hosts são permitidos);
 * - qualquer outro esquema (javascript:, data:, file:) ou path traversal é rejeitado.
 */
export type AssetResolution =
  | { kind: "static"; src: string }
  | { kind: "remote"; src: string }
  | { kind: "invalid"; reason: string };

const SAFE_RELATIVE_PATH = /^[A-Za-z0-9_\-./]+$/;

export function resolveAssetRef(ref: string, toStatic: (path: string) => string): AssetResolution {
  const trimmed = ref.trim();
  if (trimmed.length === 0) {
    return { kind: "invalid", reason: "referência vazia" };
  }

  if (/^https?:\/\//i.test(trimmed)) {
    try {
      const url = new URL(trimmed);
      return { kind: "remote", src: url.toString() };
    } catch {
      return { kind: "invalid", reason: "URL malformada" };
    }
  }

  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) {
    return { kind: "invalid", reason: "esquema não permitido" };
  }

  const normalized = trimmed.replace(/^\/+/, "");
  if (!SAFE_RELATIVE_PATH.test(normalized) || normalized.split("/").includes("..")) {
    return { kind: "invalid", reason: "caminho relativo inválido" };
  }

  return { kind: "static", src: toStatic(normalized) };
}
