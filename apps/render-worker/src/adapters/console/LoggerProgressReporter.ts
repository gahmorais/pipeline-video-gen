import type { Logger } from "pino";
import type { ProgressReporter } from "../../ports";

/** ProgressReporter para o modo CLI: apenas registra em log estruturado. */
export class LoggerProgressReporter implements ProgressReporter {
  constructor(private readonly logger: Logger) {}

  async report(jobId: string, progress: number): Promise<void> {
    this.logger.info({ job_id: jobId, progress: Math.round(progress * 100) }, "render progress");
  }
}
