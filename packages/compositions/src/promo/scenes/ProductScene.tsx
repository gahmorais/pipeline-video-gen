import type React from "react";
import { AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import type { Brand, ProductScene as ProductSceneProps } from "@videogen/video-schema";
import { FONT_STACK } from "../../shared/theme";
import { AssetImage } from "../../shared/AssetImage";
import { SceneFrame } from "../../shared/SceneFrame";

export const ProductScene: React.FC<{ scene: ProductSceneProps; brand: Brand }> = ({
  scene,
  brand,
}) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const enter = spring({ frame, fps, config: { damping: 18, mass: 0.8 } });
  const imageX = interpolate(enter, [0, 1], [-200, 0]);
  const textOpacity = interpolate(frame, [fps * 0.3, fps * 0.7], [0, 1], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <SceneFrame background="#FFFFFF">
      <AbsoluteFill
        style={{ flexDirection: "row", alignItems: "center", padding: 120, gap: 96 }}
      >
        <AssetImage
          src={scene.imageUrl}
          alt={scene.name}
          style={{ width: 760, height: 760, transform: `translateX(${imageX}px)` }}
        />
        <div style={{ flex: 1, fontFamily: FONT_STACK, opacity: textOpacity }}>
          <h2 style={{ margin: 0, fontSize: 88, fontWeight: 700, color: "#111111" }}>{scene.name}</h2>
          <p style={{ margin: "40px 0 0", fontSize: 120, fontWeight: 800, color: brand.primary }}>
            {scene.price}
          </p>
        </div>
      </AbsoluteFill>
    </SceneFrame>
  );
};
