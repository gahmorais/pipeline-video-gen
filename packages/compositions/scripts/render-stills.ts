/**
 * Validação visual: renderiza um still no meio de cada cena da fixture básica.
 *
 * Uso: pnpm render:stills [diretório-de-saída]
 * Variáveis: BROWSER_EXECUTABLE (opcional) — caminho de um Chrome local quando
 * o download do Chrome Headless Shell não for possível.
 */
import { mkdirSync, readFileSync, statSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { bundle } from "@remotion/bundler";
import { renderStill, selectComposition } from "@remotion/renderer";
import { PromoSchema, sceneFrames } from "@videogen/video-schema";

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);

async function main(): Promise<void> {
  const outDir = resolve(process.argv[2] ?? join(here, "..", "out", "stills"));
  mkdirSync(outDir, { recursive: true });

  const fixture = require.resolve("@videogen/video-schema/fixtures/valid/promo-basic.json");
  const inputProps = PromoSchema.parse(JSON.parse(readFileSync(fixture, "utf8")));

  const browserExecutable = process.env.BROWSER_EXECUTABLE ?? null;
  const chromeMode = browserExecutable ? "chrome-for-testing" : "headless-shell";

  const serveUrl = await bundle({ entryPoint: join(here, "..", "src", "index.ts") });
  const composition = await selectComposition({
    serveUrl,
    id: "Promo",
    inputProps,
    browserExecutable,
    chromeMode,
  });

  const frames = sceneFrames(inputProps.scenes, composition.fps);
  let start = 0;
  for (const [index, sceneLength] of frames.entries()) {
    const frame = start + Math.floor(sceneLength / 2);
    const output = join(outDir, `scene-${index + 1}-${inputProps.scenes[index]?.type}.png`);
    await renderStill({
      composition,
      serveUrl,
      inputProps,
      frame,
      output,
      browserExecutable,
      chromeMode,
    });
    const { size } = statSync(output);
    if (size < 1024) {
      throw new Error(`still suspeito (${size} bytes): ${output}`);
    }
    console.log(`ok frame=${frame} ${output} (${size} bytes)`);
    start += sceneLength;
  }
  console.log(`durationInFrames=${composition.durationInFrames}`);
}

main().catch((err: unknown) => {
  console.error(err);
  process.exit(1);
});
