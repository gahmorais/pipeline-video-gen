/**
 * Modelo de domínio do render worker. Sem dependência de Remotion, FS ou fila.
 */

export interface RenderRequest {
  jobId: string;
  /** Identifica uma tentativa de render; determina o caminho do arquivo (idempotência). */
  renderId: string;
  templateId: string;
  templateVersion: number;
  /** Storyboard ainda não confiável: validado pelo use case antes do render. */
  storyboard: unknown;
}

export interface RenderResult {
  jobId: string;
  renderId: string;
  outputKey: string;
  sizeBytes: number;
  /** true quando o arquivo já existia e o render foi pulado. */
  reused: boolean;
}

const ID_PATTERN = /^[A-Za-z0-9_-]{1,64}$/;

export function isValidId(value: string): boolean {
  return ID_PATTERN.test(value);
}

/** Chave do arquivo no storage: media/{jobId}/{renderId}.mp4 */
export function outputKeyFor(jobId: string, renderId: string): string {
  return `${jobId}/${renderId}.mp4`;
}
