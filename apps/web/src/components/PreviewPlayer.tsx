"use client";

import { Player } from "@remotion/player";
import { compositions } from "@videogen/compositions";
import { totalDurationInFrames, type StoryboardOf, type TemplateId } from "@videogen/video-schema";
import type React from "react";
import { useMemo } from "react";

/**
 * Preview no browser com a MESMA composição usada pelo render worker
 * (registro compartilhado): o que o usuário vê é o que será renderizado — sem custo.
 */
export function PreviewPlayer<T extends TemplateId>({
  templateId,
  storyboard,
}: {
  templateId: T;
  storyboard: StoryboardOf<T>;
}) {
  const { component, config } = compositions[templateId];
  const durationInFrames = useMemo(
    () => totalDurationInFrames(storyboard.scenes, config.fps),
    [storyboard.scenes, config.fps],
  );
  const vertical = config.height > config.width;

  return (
    <div style={{ width: "100%", maxWidth: vertical ? 380 : undefined, margin: "0 auto" }}>
      <Player
        // O registro garante que componente e props pertencem ao mesmo template.
        component={component as React.FC<Record<string, unknown>>}
        inputProps={storyboard as Record<string, unknown>}
        durationInFrames={durationInFrames}
        fps={config.fps}
        compositionWidth={config.width}
        compositionHeight={config.height}
        style={{ width: "100%", aspectRatio: `${config.width} / ${config.height}` }}
        // Começa em ~1 s: o frame 0 é o início do fade-in (tela vazia).
        initialFrame={Math.min(durationInFrames - 1, config.fps)}
        controls
        loop
      />
    </div>
  );
}
