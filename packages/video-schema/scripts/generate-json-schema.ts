import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { templates, toJsonSchema, type TemplateId } from "../src/index.js";

const here = dirname(fileURLToPath(import.meta.url));
const packageRoot = join(here, "..");
const repoRoot = join(packageRoot, "..", "..");

/**
 * Destinos do JSON Schema gerado:
 * - packages/video-schema/schemas: referência canônica
 * - apps/api/internal/adapters/schema/schemas: cópia embutida via go:embed
 *   (go:embed não acessa arquivos fora do módulo Go)
 */
const outputDirs = [
  join(packageRoot, "schemas"),
  join(repoRoot, "apps", "api", "internal", "adapters", "schema", "schemas"),
];

export function schemaFileName(id: TemplateId): string {
  return `${templates[id].id}.v${templates[id].version}.schema.json`;
}

export function renderSchema(id: TemplateId): string {
  return `${JSON.stringify(toJsonSchema(id), null, 2)}\n`;
}

function main(): void {
  for (const id of Object.keys(templates) as TemplateId[]) {
    const content = renderSchema(id);
    for (const dir of outputDirs) {
      mkdirSync(dir, { recursive: true });
      const file = join(dir, schemaFileName(id));
      writeFileSync(file, content);
      console.log(`gerado: ${file}`);
    }
  }
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  main();
}
