import type React from "react";
import { useMemo } from "react";
import { AbsoluteFill, Series } from "remotion";
import { sceneFrames, type PromoStoryboard, type Scene, type Brand } from "@videogen/video-schema";
import { VIDEO_CONFIG } from "../shared/video-config";
import { CtaScene } from "./scenes/CtaScene";
import { ProductScene } from "./scenes/ProductScene";
import { TitleScene } from "./scenes/TitleScene";

const SceneRenderer: React.FC<{ scene: Scene; brand: Brand }> = ({ scene, brand }) => {
  switch (scene.type) {
    case "title":
      return <TitleScene scene={scene} brand={brand} />;
    case "product":
      return <ProductScene scene={scene} brand={brand} />;
    case "cta":
      return <CtaScene scene={scene} brand={brand} />;
    default: {
      const exhaustive: never = scene;
      return exhaustive;
    }
  }
};

export const Promo: React.FC<PromoStoryboard> = ({ brand, scenes }) => {
  // Mesma função usada no calculateMetadata: garante que a soma das sequências = duração total.
  const frames = useMemo(() => sceneFrames(scenes, VIDEO_CONFIG.fps), [scenes]);

  return (
    <AbsoluteFill style={{ backgroundColor: "#000000" }}>
      <Series>
        {scenes.map((scene, index) => (
          <Series.Sequence
            // Cenas não têm id; o índice é estável dentro de um storyboard imutável durante o render.
            key={index}
            durationInFrames={frames[index] ?? 1}
            name={`${index + 1}. ${scene.type}`}
          >
            <SceneRenderer scene={scene} brand={brand} />
          </Series.Sequence>
        ))}
      </Series>
    </AbsoluteFill>
  );
};
