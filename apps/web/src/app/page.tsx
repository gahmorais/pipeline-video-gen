import { CreateVideoForm } from "@/components/CreateVideoForm";

export default function HomePage() {
  return (
    <div className="stack narrow">
      <h1>Novo vídeo</h1>
      <p className="muted">
        A IA gera o roteiro; você revisa no preview, ajusta e só então renderiza o MP4.
      </p>
      <CreateVideoForm />
    </div>
  );
}
