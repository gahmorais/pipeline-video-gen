# Exemplos de storyboard

Convenção de nome: `<nome>.<templateId>.json`. Todo exemplo é validado nos testes do `video-schema` contra o template do nome.

## `retro99.app-promo.json` — Retro 99, 60 s, vertical

Divulgação do app Retro 99 para Reels, TikTok e Shorts: gancho com gameplay nos 3 primeiros segundos, seis jogos em sequência, dois diferenciais, pergunta de engajamento e CTA para a Google Play.

> Os jogos (corrida, blocos, cobrinha, tanque, quebra-tijolos, tiro ao alvo) são exemplos: troque nomes, legendas e clipes pelos jogos reais do app.

### Assets necessários

Coloque em `packages/compositions/public/assets/retro99/` (a pasta `public/assets` é copiada para o frontend pelo `sync-assets`):

| Arquivo | Uso | Especificação |
|---|---|---|
| `icone.png` | Abertura e CTA | PNG quadrado, 1024×1024 (ícone "Selo LCD") |
| `trilha-chiptune.mp3` | Trilha do vídeo inteiro | **≥ 60 s** (sem loop: trilha curta fica muda no fim), licença própria ou royalty-free |
| `gameplay-corrida.mp4` | Gancho (0–3 s) e cena de corrida (`startSec: 3`) | **≥ 8 s** |
| `gameplay-blocos.mp4`, `gameplay-cobrinha.mp4`, `gameplay-tanque.mp4`, `gameplay-tijolos.mp4`, `gameplay-tiro.mp4` | Uma cena de 5 s cada | **≥ 5 s** cada |
| `tela-menu.png`, `tela-lcd.png` | Cenas `feature` | Screenshot vertical, 1080×1920 |

Clipes: vertical 1080×1920, 30 fps, H.264. Cada clipe precisa durar pelo menos `startSec + durationSec` da cena; se for mais curto, o vídeo **congela no último frame**.

### Gravando o gameplay

Android (dispositivo ou emulador via USB/ADB):

```bash
adb shell screenrecord --size 1080x1920 --bit-rate 8000000 --time-limit 30 /sdcard/corrida.mp4
adb pull /sdcard/corrida.mp4 packages/compositions/public/assets/retro99/gameplay-corrida.mp4
```

Se o aparelho tiver outra resolução, normalize com FFmpeg:

```bash
ffmpeg -i entrada.mp4 -vf "scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2,fps=30" \
  -c:v libx264 -crf 20 -an -movflags +faststart saida.mp4
```

O áudio dos clipes é descartado no render (a trilha é global), então `-an` não perde nada.

### Renderizando

Direto pelo CLI do render worker (sem API):

```bash
cd apps/render-worker
pnpm render ../../docs/examples/retro99.app-promo.json --template app-promo
```

Ou pela interface: crie um vídeo no formato **Divulgação de app**, cole o JSON no editor, salve e renderize.
