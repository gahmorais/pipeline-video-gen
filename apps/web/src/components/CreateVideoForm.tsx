"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { api, ApiError } from "@/lib/api";

const MAX_PROMPT = 2000;

export function CreateVideoForm() {
  const router = useRouter();
  const [prompt, setPrompt] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    if (!prompt.trim()) {
      setError("Descreva o vídeo que você quer gerar.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const job = await api.createVideo(prompt.trim());
      router.push(`/videos/${job.id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Não foi possível falar com a API.");
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="card stack">
      <label htmlFor="prompt" className="label">
        O que o vídeo deve mostrar?
      </label>
      <textarea
        id="prompt"
        rows={5}
        maxLength={MAX_PROMPT}
        value={prompt}
        onChange={(e) => {
          setPrompt(e.target.value);
          setError(null);
        }}
        placeholder="Ex.: vídeo de 15 segundos com as ofertas da semana de leitores de código de barras, cor azul"
      />
      <div className="row between">
        <span className="muted">
          {prompt.length}/{MAX_PROMPT}
        </span>
        <button type="submit" disabled={submitting}>
          {submitting ? "Criando…" : "Gerar roteiro"}
        </button>
      </div>
      {error && <p className="error">{error}</p>}
    </form>
  );
}
