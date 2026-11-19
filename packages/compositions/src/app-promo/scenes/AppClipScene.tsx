import type React from "react";
import { AbsoluteFill, OffthreadVideo, interpolate, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import type { AppBrand, AppClipScene as Props } from "@videogen/video-schema";
import { resolveAssetRef } from "../../shared/assets";
import { SceneFrame } from "../../shared/SceneFrame";
import { FONT_STACK, readableTextColor } from "../../shared/theme";
import { VERTICAL_SAFE_AREA } from "../../shared/video-config";

/**
 * Trecho de gameplay em tela cheia. OffthreadVideo extrai o frame exato via FFmpeg
 * no render (determinístico). O áudio do clipe é silenciado: a trilha é global.
 */
export const AppClipScene: React.FC<{ scene: Props; brand: AppBrand }> = ({ scene, brand }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const resolved = resolveAssetRef(scene.videoUrl, staticFile);
  const captionY = interpolate(frame, [0, fps * 0.4], [40, 0], { extrapolateRight: "clamp" });

  return (
    <SceneFrame background="#000000">
      {resolved.kind === "invalid" ? (
        <AbsoluteFill style={{ backgroundColor: "rgba(127,127,127,0.25)" }} />
      ) : (
        <OffthreadVideo
          src={resolved.src}
          trimBefore={Math.round(scene.startSec * fps)}
          muted
          style={{ width: "100%", height: "100%", objectFit: "cover" }}
        />
      )}
      {scene.caption && (
        <AbsoluteFill
          style={{
            justifyContent: "flex-end",
            alignItems: "center",
            paddingBottom: VERTICAL_SAFE_AREA.bottom,
          }}
        >
          <div
            style={{
              transform: `translateY(${captionY}px)`,
              backgroundColor: brand.primary,
              color: readableTextColor(brand.primary),
              fontFamily: FONT_STACK,
              fontSize: 64,
              fontWeight: 800,
              padding: "20px 44px",
              borderRadius: 24,
              maxWidth: 1080 - VERTICAL_SAFE_AREA.side * 2,
              textAlign: "center",
            }}
          >
            {scene.caption}
          </div>
        </AbsoluteFill>
      )}
    </SceneFrame>
  );
};
