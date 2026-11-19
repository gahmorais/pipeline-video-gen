"use client";

import { Player } from "@remotion/player";
import { Promo, VIDEO_CONFIG } from "@videogen/compositions";
import { totalDurationInFrames, type PromoStoryboard } from "@videogen/video-schema";
import { useMemo } from "react";

/**
 * Preview no browser com a MESMA composição usada pelo render worker:
 * o que o usuário vê aqui é o que será renderizado — sem custo de render.
 */
export function PreviewPlayer({ storyboard }: { storyboard: PromoStoryboard }) {
  const durationInFrames = useMemo(
    () => totalDurationInFrames(storyboard.scenes, VIDEO_CONFIG.fps),
    [storyboard.scenes],
  );

  return (
    <Player
      component={Promo}
      inputProps={storyboard}
      durationInFrames={durationInFrames}
      fps={VIDEO_CONFIG.fps}
      compositionWidth={VIDEO_CONFIG.width}
      compositionHeight={VIDEO_CONFIG.height}
      style={{ width: "100%", aspectRatio: `${VIDEO_CONFIG.width} / ${VIDEO_CONFIG.height}` }}
      // Começa em ~1 s: o frame 0 é o início do fade-in (tela vazia).
      initialFrame={Math.min(durationInFrames - 1, VIDEO_CONFIG.fps)}
      controls
      loop
    />
  );
}
