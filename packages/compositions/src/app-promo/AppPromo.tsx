import type React from "react";
import { useMemo } from "react";
import { AbsoluteFill, Audio, Series, staticFile, useVideoConfig } from "remotion";
import { sceneFrames, type AppBrand, type AppPromoStoryboard, type AppScene } from "@videogen/video-schema";
import { resolveAssetRef } from "../shared/assets";
import { APP_PROMO_CONFIG } from "../shared/video-config";
import { soundtrackVolume } from "./audio";
import { AppClipScene } from "./scenes/AppClipScene";
import { AppCtaScene } from "./scenes/AppCtaScene";
import { AppFeatureScene } from "./scenes/AppFeatureScene";
import { AppTitleScene } from "./scenes/AppTitleScene";

const SceneRenderer: React.FC<{ scene: AppScene; brand: AppBrand }> = ({ scene, brand }) => {
  switch (scene.type) {
    case "title":
      return <AppTitleScene scene={scene} brand={brand} />;
    case "clip":
      return <AppClipScene scene={scene} brand={brand} />;
    case "feature":
      return <AppFeatureScene scene={scene} brand={brand} />;
    case "cta":
      return <AppCtaScene scene={scene} brand={brand} />;
    default: {
      const exhaustive: never = scene;
      return exhaustive;
    }
  }
};

export const AppPromo: React.FC<AppPromoStoryboard> = ({ brand, scenes, soundtrack }) => {
  const { durationInFrames } = useVideoConfig();
  const frames = useMemo(() => sceneFrames(scenes, APP_PROMO_CONFIG.fps), [scenes]);
  const track = soundtrack ? resolveAssetRef(soundtrack.src, staticFile) : null;

  return (
    <AbsoluteFill style={{ backgroundColor: "#000000" }}>
      {track && track.kind !== "invalid" && soundtrack && (
        // Sem `loop`: com loop, o frame passado ao callback de volume reinicia a cada
        // volta da trilha e o fade-out final se perde. A trilha deve durar >= o vídeo.
        <Audio
          src={track.src}
          volume={(f) => soundtrackVolume(f, durationInFrames, APP_PROMO_CONFIG.fps, soundtrack.volume)}
        />
      )}
      <Series>
        {scenes.map((scene, index) => (
          <Series.Sequence key={index} durationInFrames={frames[index] ?? 1} name={`${index + 1}. ${scene.type}`}>
            <SceneRenderer scene={scene} brand={brand} />
          </Series.Sequence>
        ))}
      </Series>
    </AbsoluteFill>
  );
};
