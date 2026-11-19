// Copia os assets das composições para public/ do Next: o Player resolve
// staticFile("assets/x.png") como "/assets/x.png", igual ao render worker.
import { cpSync, mkdirSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const compositionsRoot = dirname(require.resolve("@videogen/compositions/package.json"));
const target = join(dirname(fileURLToPath(import.meta.url)), "..", "public", "assets");

mkdirSync(target, { recursive: true });
cpSync(join(compositionsRoot, "public", "assets"), target, { recursive: true });
console.log(`assets sincronizados em ${target}`);
