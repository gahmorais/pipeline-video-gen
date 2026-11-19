"use client";

import { useEffect, useState } from "react";
import { api, type VideoDto } from "./api";

export type ConnectionState = "connecting" | "open" | "reconnecting";

/**
 * Estado do job via SSE. Cada evento "job" é um snapshot completo (não delta),
 * então o hook só substitui o estado. O EventSource reconecta sozinho.
 * Snapshots com versão menor que a atual são ignorados (respostas fora de ordem).
 */
export function useVideoJob(id: string): {
  job: VideoDto | null;
  connection: ConnectionState;
  /** Aplica um snapshot vindo de uma resposta HTTP (ex.: após PATCH), sem esperar o SSE. */
  apply: (job: VideoDto) => void;
} {
  const [job, setJob] = useState<VideoDto | null>(null);
  const [connection, setConnection] = useState<ConnectionState>("connecting");

  const apply = (next: VideoDto) => {
    setJob((current) => (current && current.version > next.version ? current : next));
  };

  useEffect(() => {
    const source = new EventSource(api.eventsUrl(id));
    source.addEventListener("open", () => setConnection("open"));
    source.addEventListener("error", () => setConnection("reconnecting"));
    source.addEventListener("job", (event) => {
      try {
        apply(JSON.parse((event as MessageEvent<string>).data) as VideoDto);
      } catch {
        // snapshot malformado: ignora, o próximo corrige
      }
    });
    return () => source.close();
  }, [id]);

  return { job, connection, apply };
}
