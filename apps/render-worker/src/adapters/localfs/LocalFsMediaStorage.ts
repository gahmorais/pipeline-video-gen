import { randomUUID } from "node:crypto";
import { mkdir, rename, rm, stat } from "node:fs/promises";
import { dirname, resolve, sep } from "node:path";
import type { MediaStorage, WriteTarget } from "../../ports";

/**
 * MediaStorage em filesystem local (./media).
 * Escreve em `<key>.partial-<uuid>.mp4` e faz rename atômico no commit,
 * então `exists()` nunca vê arquivo pela metade.
 */
export class LocalFsMediaStorage implements MediaStorage {
  private readonly root: string;

  constructor(rootDir: string) {
    this.root = resolve(rootDir);
  }

  async exists(key: string): Promise<boolean> {
    try {
      const info = await stat(this.pathFor(key));
      return info.isFile() && info.size > 0;
    } catch (err) {
      if (isNotFound(err)) {
        return false;
      }
      throw err;
    }
  }

  async createWriteTarget(key: string): Promise<WriteTarget> {
    const finalPath = this.pathFor(key);
    await mkdir(dirname(finalPath), { recursive: true });
    const localPath = `${finalPath}.partial-${randomUUID()}.mp4`;

    return {
      localPath,
      commit: async () => {
        const info = await stat(localPath);
        if (info.size === 0) {
          throw new Error(`arquivo renderizado vazio: ${localPath}`);
        }
        await rename(localPath, finalPath);
        return { sizeBytes: info.size };
      },
      abort: async () => {
        await rm(localPath, { force: true });
      },
    };
  }

  /** Resolve a chave dentro da raiz, bloqueando path traversal. */
  pathFor(key: string): string {
    const full = resolve(this.root, key);
    if (!full.startsWith(this.root + sep)) {
      throw new Error(`chave fora do diretório de mídia: ${key}`);
    }
    return full;
  }
}

function isNotFound(err: unknown): boolean {
  return typeof err === "object" && err !== null && "code" in err && err.code === "ENOENT";
}
