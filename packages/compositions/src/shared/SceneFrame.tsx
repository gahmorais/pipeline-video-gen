import type React from "react";
import { AbsoluteFill, interpolate, useCurrentFrame, useVideoConfig } from "remotion";
import { fadeFrames } from "./theme";

/**
 * Envelope comum das cenas: fundo + fade-in/fade-out derivado do frame local.
 * Dentro de <Series.Sequence>, useCurrentFrame() começa em 0 e
 * useVideoConfig().durationInFrames é a duração da própria cena.
 */
export const SceneFrame: React.FC<{ background: string; children: React.ReactNode }> = ({
  background,
  children,
}) => {
  const frame = useCurrentFrame();
  const { durationInFrames, fps } = useVideoConfig();
  const fade = fadeFrames(durationInFrames, fps);

  const opacity = interpolate(
    frame,
    [0, fade, durationInFrames - fade, durationInFrames],
    [0, 1, 1, 0],
    { extrapolateLeft: "clamp", extrapolateRight: "clamp" },
  );

  return (
    <AbsoluteFill style={{ backgroundColor: background }}>
      <AbsoluteFill style={{ opacity }}>{children}</AbsoluteFill>
    </AbsoluteFill>
  );
};
