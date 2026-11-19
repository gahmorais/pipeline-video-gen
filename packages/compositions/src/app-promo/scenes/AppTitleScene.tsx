import type React from "react";
import { AbsoluteFill, spring, useCurrentFrame, useVideoConfig } from "remotion";
import type { AppBrand, AppTitleScene as Props } from "@videogen/video-schema";
import { AssetImage } from "../../shared/AssetImage";
import { SceneFrame } from "../../shared/SceneFrame";
import { FONT_STACK, readableTextColor } from "../../shared/theme";
import { VERTICAL_SAFE_AREA } from "../../shared/video-config";

export const AppTitleScene: React.FC<{ scene: Props; brand: AppBrand }> = ({ scene, brand }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const pop = spring({ frame, fps, config: { damping: 14, mass: 0.7 } });

  return (
    <SceneFrame background={brand.primary}>
      <AbsoluteFill
        style={{
          justifyContent: "center",
          alignItems: "center",
          gap: 64,
          padding: `${VERTICAL_SAFE_AREA.top}px ${VERTICAL_SAFE_AREA.side}px ${VERTICAL_SAFE_AREA.bottom}px`,
        }}
      >
        <AssetImage
          src={brand.logoUrl}
          alt={brand.appName}
          style={{ width: 360, height: 360, borderRadius: 72, transform: `scale(${0.6 + 0.4 * pop})` }}
        />
        <h1
          style={{
            margin: 0,
            textAlign: "center",
            fontFamily: FONT_STACK,
            fontSize: 104,
            lineHeight: 1.1,
            textWrap: "balance",
            fontWeight: 800,
            color: readableTextColor(brand.primary),
          }}
        >
          {scene.text}
        </h1>
      </AbsoluteFill>
    </SceneFrame>
  );
};
