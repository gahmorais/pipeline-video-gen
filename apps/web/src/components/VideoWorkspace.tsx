"use client";

import { totalDurationSec, type PromoStoryboard, type ValidationIssue } from "@videogen/video-schema";
import { useEffect, useMemo, useRef, useState } from "react";
import { api, ApiError, newIdempotencyKey, type JobStatus, type VideoDto } from "@/lib/api";
import { estimateRemainingSeconds, formatDuration } from "@/lib/eta";
import { parseStoryboard, sameStoryboard, validate } from "@/lib/storyboard-editor";
import { useVideoJob } from "@/lib/use-video-job";
import { PreviewPlayer } from "./PreviewPlayer";
import { StoryboardEditor } from "./StoryboardEditor";

const STATUS_LABEL: Record<JobStatus, string> = {
  DRAFT: "Na fila para gerar o roteiro",
  GENERATING_STORYBOARD: "Gerando roteiro com IA…",
  STORYBOARD_READY: "Roteiro pronto — revise e renderize",
  RENDER_QUEUED: "Na fila de render",
  RENDERING: "Renderizando",
  COMPLETED: "Vídeo pronto",
  FAILED: "Falhou",
};

const EDITABLE: JobStatus[] = ["STORYBOARD_READY", "COMPLETED"];

export function VideoWorkspace({ id }: { id: string }) {
  const { job, connection, apply } = useVideoJob(id);
  const [draft, setDraft] = useState<PromoStoryboard | null>(null);
  const [serverIssues, setServerIssues] = useState<ValidationIssue[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Uma chave por intenção de render: reenvios (duplo clique, retry de rede) não duplicam.
  const renderKey = useRef<string | null>(null);

  const saved = useMemo(() => (job?.storyboard ? parseStoryboard(job.storyboard) : null), [job?.storyboard]);
  const dirty = draft !== null && saved !== null && !sameStoryboard(draft, saved);

  // Sincroniza o rascunho com o servidor, sem atropelar edições não salvas.
  useEffect(() => {
    if (saved && !dirty) {
      setDraft(saved);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [saved]);

  const clientIssues = useMemo(() => (draft ? validate(draft) : []), [draft]);
  const issues = clientIssues.length > 0 ? clientIssues : serverIssues;
  const editable = job !== null && EDITABLE.includes(job.status);

  async function run(action: () => Promise<VideoDto>) {
    setBusy(true);
    setError(null);
    try {
      apply(await action());
      setServerIssues([]);
    } catch (err) {
      if (err instanceof ApiError && err.issues.length > 0) {
        setServerIssues(err.issues);
      }
      setError(err instanceof ApiError ? err.message : "Falha de comunicação com a API.");
    } finally {
      setBusy(false);
    }
  }

  const save = () => draft && run(() => api.updateStoryboard(id, draft));
  const render = () => {
    renderKey.current ??= newIdempotencyKey();
    const key = renderKey.current;
    return run(async () => {
      const result = await api.requestRender(id, key);
      renderKey.current = null;
      return result;
    });
  };
  const retry = () => run(() => api.retry(id));

  if (!job) {
    return <p className="muted">Conectando…</p>;
  }

  return (
    <div className="workspace">
      <header className="stack">
        <div className="row between">
          <span className={`badge status-${job.status.toLowerCase()}`}>{STATUS_LABEL[job.status]}</span>
          {connection !== "open" && <span className="muted">reconectando…</span>}
        </div>
        <p className="prompt">“{job.prompt}”</p>
      </header>

      {(job.status === "DRAFT" || job.status === "GENERATING_STORYBOARD") && <div className="spinner" aria-label="Carregando" />}

      {job.status === "FAILED" && job.failure && (
        <div className="card error-box stack">
          <p>
            <strong>Erro:</strong> {job.failure.reason}
          </p>
          <button onClick={retry} disabled={busy}>
            Tentar novamente
          </button>
        </div>
      )}

      {draft && (
        <div className="grid">
          <section className="stack">
            <PreviewPlayer storyboard={clientIssues.length === 0 ? draft : (saved ?? draft)} />
            {clientIssues.length > 0 && <p className="muted">Preview mostra a última versão válida.</p>}

            <RenderPanel
              job={job}
              busy={busy}
              dirty={dirty}
              onRender={render}
              videoSeconds={saved ? totalDurationSec(saved.scenes) : 0}
            />
          </section>

          <section className="stack">
            <div className="row between">
              <h2>Roteiro</h2>
              <div className="row">
                {dirty && (
                  <button className="ghost" onClick={() => saved && setDraft(saved)} disabled={busy}>
                    Descartar
                  </button>
                )}
                <button onClick={save} disabled={!editable || !dirty || busy || clientIssues.length > 0}>
                  Salvar
                </button>
              </div>
            </div>
            <StoryboardEditor draft={draft} issues={issues} disabled={!editable || busy} onChange={setDraft} />
          </section>
        </div>
      )}

      {error && <p className="error">{error}</p>}
    </div>
  );
}

interface RenderPanelProps {
  job: VideoDto;
  busy: boolean;
  dirty: boolean;
  onRender: () => void;
  videoSeconds: number;
}

function RenderPanel({ job, busy, dirty, onRender, videoSeconds }: RenderPanelProps) {
  const rendering = job.status === "RENDER_QUEUED" || job.status === "RENDERING";
  const progress = Math.round((job.render?.progress ?? 0) * 100);
  const [ratio, setRatio] = useState(0);

  // Histórico real de render alimenta o ETA (busca uma vez por render).
  useEffect(() => {
    if (!rendering) return;
    let cancelled = false;
    api
      .getMetrics()
      .then((m) => !cancelled && setRatio(m.render.renderSecondsPerVideoSecond))
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [rendering]);

  if (rendering) {
    const eta = job.status === "RENDERING" ? estimateRemainingSeconds(job.render?.progress ?? 0, videoSeconds, ratio) : null;
    return (
      <div className="card stack">
        <progress max={100} value={job.status === "RENDERING" ? progress : undefined} />
        <span className="muted">
          {job.status === "RENDERING" ? `${progress}%` : "Aguardando o render worker…"}
          {eta !== null && ` · ~${formatDuration(eta)} restantes`}
        </span>
      </div>
    );
  }

  const mediaUrl = job.status === "COMPLETED" && job.mediaUrl ? api.absoluteUrl(job.mediaUrl) : null;
  return (
    <div className="card stack">
      {mediaUrl && (
        <>
          <video src={mediaUrl} controls className="video" />
          <a href={mediaUrl} download={`${job.id}.mp4`}>
            Baixar MP4
          </a>
        </>
      )}
      {(job.status === "STORYBOARD_READY" || job.status === "COMPLETED") && (
        <button onClick={onRender} disabled={busy || dirty} title={dirty ? "Salve o roteiro antes de renderizar" : undefined}>
          {mediaUrl ? "Renderizar novamente" : "Renderizar vídeo"}
        </button>
      )}
    </div>
  );
}
