import { createServer, type IncomingMessage, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { afterEach, describe, expect, it } from "vitest";
import { HttpJobGateway } from "../src/adapters/api-gateway/HttpJobGateway";
import { RenderFailedError } from "../src/domain/errors";

interface Recorded {
  method: string;
  url: string;
  auth: string | undefined;
  body: string;
}

let server: Server | null = null;

afterEach(async () => {
  await new Promise<void>((r) => (server ? server.close(() => r()) : r()));
  server = null;
});

async function start(status: number, response: unknown): Promise<{ gateway: HttpJobGateway; requests: Recorded[] }> {
  const requests: Recorded[] = [];
  server = createServer(async (req: IncomingMessage, res) => {
    let body = "";
    for await (const chunk of req) body += chunk;
    requests.push({ method: req.method ?? "", url: req.url ?? "", auth: req.headers.authorization, body });
    res.writeHead(status, { "Content-Type": "application/json" });
    res.end(JSON.stringify(response));
  });
  await new Promise<void>((r) => server?.listen(0, "127.0.0.1", () => r()));
  const { port } = server.address() as AddressInfo;
  const gateway = new HttpJobGateway({ baseUrl: `http://127.0.0.1:${port}`, internalToken: "tok", timeoutMs: 2000 });
  return { gateway, requests };
}

describe("HttpJobGateway", () => {
  it("start devolve template e storyboard do DTO", async () => {
    const { gateway, requests } = await start(200, {
      status: "RENDERING",
      template: { id: "promo", version: 1 },
      storyboard: { schemaVersion: 1 },
    });
    const result = await gateway.startRender("job 1", "r1");
    expect(result).toEqual({
      kind: "started",
      job: { templateId: "promo", templateVersion: 1, storyboard: { schemaVersion: 1 } },
    });
    expect(requests[0]).toMatchObject({
      method: "POST",
      url: "/internal/videos/job%201/renders/r1/start",
      auth: "Bearer tok",
    });
  });

  it.each(["render-mismatch", "invalid-transition", "not-found"])("start com %s vira stale", async (type) => {
    const { gateway } = await start(409, { type, detail: "detalhe" });
    expect(await gateway.startRender("j", "r")).toEqual({ kind: "stale", reason: "detalhe" });
  });

  it("start com 500 lança erro retentável", async () => {
    const { gateway } = await start(500, { type: "internal" });
    await expect(gateway.startRender("j", "r")).rejects.toBeInstanceOf(RenderFailedError);
  });

  it("progress, complete e fail enviam o corpo esperado", async () => {
    const { gateway, requests } = await start(200, {});
    await gateway.report("j", "r", 0.25);
    await gateway.completeRender("j", "r", "j/r.mp4");
    await gateway.failRender("j", "r", "boom");
    expect(requests.map((r) => [r.url, JSON.parse(r.body)])).toEqual([
      ["/internal/videos/j/renders/r/progress", { progress: 0.25 }],
      ["/internal/videos/j/renders/r/complete", { outputKey: "j/r.mp4" }],
      ["/internal/videos/j/renders/r/fail", { reason: "boom" }],
    ]);
  });

  it("erro em complete propaga com o tipo do problema", async () => {
    const { gateway } = await start(404, { type: "not-found", detail: "mídia ausente" });
    await expect(gateway.completeRender("j", "r", "k")).rejects.toThrow(/404 not-found mídia ausente/);
  });
});
