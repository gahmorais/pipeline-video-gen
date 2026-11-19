import type React from "react";
import { AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import type { AppBrand, AppFeatureScene as Props } from "@videogen/video-schema";
import { AssetImage } from "../../shared/AssetImage";
import { SceneFrame } from "../../shared/SceneFrame";
import { FONT_STACK } from "../../shared/theme";
import { VERTICAL_SAFE_AREA } from "../../shared/video-config";

/** Screenshot em "moldura de celular" com nome e frase do recurso. */
export const AppFeatureScene: React.FC<{ scene: Props; brand: AppBrand }> = ({ scene, brand }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const rise = spring({ frame, fps, config: { damping: 16 } });
  const textOpacity = interpolate(frame, [fps * 0.25, fps * 0.6], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <SceneFrame background="#101410">
      <AbsoluteFill
        style={{
          alignItems: "center",
          justifyContent: "center",
          gap: 56,
          padding: `${VERTICAL_SAFE_AREA.top}px ${VERTICAL_SAFE_AREA.side}px ${VERTICAL_SAFE_AREA.bottom}px`,
        }}
      >
        <div style={{ fontFamily: FONT_STACK, textAlign: "center", opacity: textOpacity }}>
          <h2 style={{ margin: 0, fontSize: 96, fontWeight: 800, color: brand.primary }}>{scene.name}</h2>
          <p style={{ margin: "16px 0 0", fontSize: 56, fontWeight: 600, color: "#F2F2F2" }}>{scene.tagline}</p>
        </div>
        <div
          style={{
            width: 620,
            height: 900,
            borderRadius: 64,
            border: `14px solid ${brand.primary}`,
            overflow: "hidden",
            backgroundColor: "#000000",
            transform: `translateY(${interpolate(rise, [0, 1], [160, 0])}px)`,
          }}
        >
          <AssetImage src={scene.imageUrl} alt={scene.name} style={{ width: "100%", height: "100%", objectFit: "cover" }} />
        </div>
      </AbsoluteFill>
    </SceneFrame>
  );
};
