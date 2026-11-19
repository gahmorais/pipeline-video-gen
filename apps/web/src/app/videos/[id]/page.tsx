import { notFound } from "next/navigation";
import { VideoWorkspace } from "@/components/VideoWorkspace";

const ID_PATTERN = /^[A-Za-z0-9_-]{1,64}$/;

export default async function VideoPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  if (!ID_PATTERN.test(id)) {
    notFound();
  }
  return <VideoWorkspace id={id} />;
}
