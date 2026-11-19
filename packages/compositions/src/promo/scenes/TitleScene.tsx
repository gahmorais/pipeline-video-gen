import type React from "react";
import { AbsoluteFill, spring, useCurrentFrame, useVideoConfig } from "remotion";
import type { Brand, TitleScene as TitleSceneProps } from "@videogen/video-schema";
import { FONT_STACK, readableTextColor } from "../theme";
import { AssetImage } from "./AssetImage";
import { SceneFrame } from "./SceneFrame";

export const TitleScene: React.FC<{ scene: TitleSceneProps; brand: Brand }> = ({ scene, brand }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const scale = spring({ frame, fps, config: { damping: 200 } });
  const color = readableTextColor(brand.primary);

  return (
    <SceneFrame background={brand.primary}>
      <AbsoluteFill style={{ justifyContent: "center", alignItems: "center", gap: 48 }}>
        <AssetImage src={brand.logoUrl} alt="logo" style={{ width: 220, height: 220 }} />
        <h1
          style={{
            margin: 0,
            padding: "0 120px",
            textAlign: "center",
            fontFamily: FONT_STACK,
            fontSize: 110,
            fontWeight: 800,
            color,
            transform: `scale(${0.8 + 0.2 * scale})`,
          }}
        >
          {scene.text}
        </h1>
      </AbsoluteFill>
    </SceneFrame>
  );
};
