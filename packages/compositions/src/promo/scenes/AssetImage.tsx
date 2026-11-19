import type React from "react";
import { Img, staticFile } from "remotion";
import { resolveAssetRef } from "../../shared/assets";

/**
 * Imagem a partir de uma referência do storyboard. Referências inválidas
 * viram um placeholder em vez de quebrar o render inteiro.
 */
export const AssetImage: React.FC<{ src: string; alt: string; style?: React.CSSProperties }> = ({
  src,
  alt,
  style,
}) => {
  const resolved = resolveAssetRef(src, staticFile);
  if (resolved.kind === "invalid") {
    return (
      <div
        role="img"
        aria-label={alt}
        style={{ ...style, backgroundColor: "rgba(127,127,127,0.25)", borderRadius: 24 }}
      />
    );
  }
  return <Img src={resolved.src} alt={alt} style={{ objectFit: "contain", ...style }} />;
};
