/**
 * Render real com Remotion + Chrome + FFmpeg. Lento: só roda com RUN_RENDER_TESTS=1.
 *   pnpm test:integration          (Chrome Headless Shell gerenciado)
 *   BROWSER_EXECUTABLE=... pnpm test:integration
 */
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { mkdtemp } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { LocalFsMediaStorage } from "../../src/adapters/localfs/LocalFsMediaStorage";
import { RemotionLocalRenderer } from "../../src/adapters/remotion-local/RemotionLocalRenderer";
import { VideoSchemaValidator } from "../../src/adapters/schema/VideoSchemaValidator";
import { RenderVideo } from "../../src/application/RenderVideo";
import { loadConfig } from "../../src/config";
import { FakeProgress } from "../fakes";

const require = createRequire(import.meta.url);
const enabled = process.env.RUN_RENDER_TESTS === "1";

describe.runIf(enabled)("render real (Remotion)", () => {
  it(
    "gera MP4 H.264 1080p30 com a duração do storyboard e reaproveita na segunda chamada",
    async () => {
      const mediaDir = await mkdtemp(join(tmpdir(), "render-it-"));
      const storage = new LocalFsMediaStorage(mediaDir);
      const progress = new FakeProgress();
      const useCase = new RenderVideo({
        renderer: new RemotionLocalRenderer(loadConfig().remotion),
        storage,
        progress,
        validator: new VideoSchemaValidator(),
      });
      const storyboard: unknown = JSON.parse(
        readFileSync(require.resolve("@videogen/video-schema/fixtures/valid/promo-minimal.json"), "utf8"),
      );
      const request = { jobId: "it-job", renderId: "it-render", templateId: "promo", templateVersion: 1, storyboard };

      const first = await useCase.execute(request);
      expect(first.reused).toBe(false);
      expect(first.sizeBytes).toBeGreaterThan(1000);
      expect(progress.events.at(-1)).toBe(1);

      const probe = JSON.parse(
        execFileSync("ffprobe", [
          "-v", "error",
          "-select_streams", "v:0",
          "-show_entries", "stream=codec_name,width,height,r_frame_rate,nb_frames",
          "-of", "json",
          storage.pathFor(first.outputKey),
        ]).toString(),
      ) as { streams: Array<Record<string, string>> };
      expect(probe.streams[0]).toMatchObject({
        codec_name: "h264",
        width: 1920,
        height: 1080,
        r_frame_rate: "30/1",
        nb_frames: "30", // promo-minimal: 1 cena de 1s a 30 fps
      });

      const second = await useCase.execute(request);
      expect(second.reused).toBe(true);
    },
    300_000,
  );
});
