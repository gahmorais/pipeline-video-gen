import type {
  MediaStorage,
  ProgressReporter,
  Renderer,
  RendererInput,
  RendererOutput,
  StoryboardValidation,
  StoryboardValidator,
  WriteTarget,
} from "../src/ports";

export class FakeRenderer implements Renderer {
  calls: RendererInput[] = [];
  constructor(
    private readonly behavior: (input: RendererInput) => Promise<RendererOutput> = async (input) => {
      for (const p of [0, 0.02, 0.07, 0.07, 0.5, 0.99, 1]) {
        input.onProgress(p);
      }
      return { durationInFrames: 30 };
    },
  ) {}

  async render(input: RendererInput): Promise<RendererOutput> {
    this.calls.push(input);
    return this.behavior(input);
  }
}

export class FakeStorage implements MediaStorage {
  readonly committed = new Set<string>();
  readonly aborted: string[] = [];

  constructor(existing: string[] = []) {
    existing.forEach((k) => this.committed.add(k));
  }

  async exists(key: string): Promise<boolean> {
    return this.committed.has(key);
  }

  async createWriteTarget(key: string): Promise<WriteTarget> {
    return {
      localPath: `/tmp/fake/${key}.partial`,
      commit: async () => {
        this.committed.add(key);
        return { sizeBytes: 1234 };
      },
      abort: async () => {
        this.aborted.push(key);
      },
    };
  }
}

export class FakeProgress implements ProgressReporter {
  readonly events: number[] = [];
  async report(_jobId: string, _renderId: string, progress: number): Promise<void> {
    this.events.push(progress);
  }
}

export class FakeValidator implements StoryboardValidator {
  constructor(private readonly result: StoryboardValidation = { valid: true, compositionId: "Promo", inputProps: { ok: true } }) {}
  validate(): StoryboardValidation {
    return this.result;
  }
}
