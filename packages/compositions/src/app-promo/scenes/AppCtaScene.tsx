import type React from "react";
import { AbsoluteFill, useCurrentFrame, useVideoConfig } from "remotion";
import type { AppBrand, AppCtaScene as Props, Store } from "@videogen/video-schema";
import { AssetImage } from "../../shared/AssetImage";
import { SceneFrame } from "../../shared/SceneFrame";
import { FONT_STACK, readableTextColor } from "../../shared/theme";
import { VERTICAL_SAFE_AREA } from "../../shared/video-config";

// Apenas texto: não reproduz logotipos/selos oficiais das lojas.
const STORE_LABEL: Record<Store, string> = {
  "google-play": "Google Play",
  "app-store": "App Store",
};

export const AppCtaScene: React.FC<{ scene: Props; brand: AppBrand }> = ({ scene, brand }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const pulse = 1 + 0.05 * Math.sin((frame / fps) * Math.PI * 2);

  return (
    <SceneFrame background="#101410">
      <AbsoluteFill
        style={{
          alignItems: "center",
          justifyContent: "center",
          gap: 72,
          padding: `${VERTICAL_SAFE_AREA.top}px ${VERTICAL_SAFE_AREA.side}px ${VERTICAL_SAFE_AREA.bottom}px`,
          fontFamily: FONT_STACK,
        }}
      >
        <AssetImage src={brand.logoUrl} alt={brand.appName} style={{ width: 260, height: 260, borderRadius: 56 }} />
        <div style={{ fontSize: 80, fontWeight: 800, color: "#FFFFFF", textAlign: "center" }}>{brand.appName}</div>
        <div
          style={{
            backgroundColor: brand.primary,
            color: readableTextColor(brand.primary),
            fontSize: 76,
            fontWeight: 800,
            padding: "36px 72px",
            borderRadius: 999,
            transform: `scale(${pulse})`,
            textAlign: "center",
            textWrap: "balance",
          }}
        >
          {scene.text}
        </div>
        <div style={{ display: "flex", gap: 28 }}>
          {scene.stores.map((store) => (
            <div
              key={store}
              style={{
                border: "4px solid #FFFFFF",
                color: "#FFFFFF",
                borderRadius: 24,
                padding: "18px 36px",
                fontSize: 44,
                fontWeight: 700,
              }}
            >
              {STORE_LABEL[store]}
            </div>
          ))}
        </div>
      </AbsoluteFill>
    </SceneFrame>
  );
};
