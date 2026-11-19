import type React from "react";
import { AbsoluteFill, useCurrentFrame, useVideoConfig } from "remotion";
import type { Brand, CtaScene as CtaSceneProps } from "@videogen/video-schema";
import { FONT_STACK, readableTextColor } from "../../shared/theme";
import { SceneFrame } from "../../shared/SceneFrame";

export const CtaScene: React.FC<{ scene: CtaSceneProps; brand: Brand }> = ({ scene, brand }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  // Pulso determinístico: função pura do frame (1 ciclo por segundo).
  const pulse = 1 + 0.04 * Math.sin((frame / fps) * Math.PI * 2);
  const color = readableTextColor(brand.primary);

  return (
    <SceneFrame background="#111111">
      <AbsoluteFill style={{ justifyContent: "center", alignItems: "center" }}>
        <div
          style={{
            backgroundColor: brand.primary,
            color,
            fontFamily: FONT_STACK,
            fontSize: 96,
            fontWeight: 800,
            padding: "48px 96px",
            borderRadius: 999,
            transform: `scale(${pulse})`,
          }}
        >
          {scene.text}
        </div>
      </AbsoluteFill>
    </SceneFrame>
  );
};
