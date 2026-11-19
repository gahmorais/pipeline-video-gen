import type { Metadata } from "next";
import Link from "next/link";
import type { ReactNode } from "react";
import "./globals.css";

export const metadata: Metadata = {
  title: "Video Gen",
  description: "Geração de vídeos promocionais com IA e Remotion",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="pt-BR">
      <body>
        <header className="topbar">
          <Link href="/">Video Gen</Link>
        </header>
        <main className="container">{children}</main>
      </body>
    </html>
  );
}
