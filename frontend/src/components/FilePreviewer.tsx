import React, { useState } from "react";
import { Button } from "@/components/ui/button";
import { getFileKind } from "@/lib/fileKind";

type Props = {
  url: string;
  filename: string;
  mimeType?: string;
};

export default function FilePreviewer({ url, filename, mimeType }: Props) {
  const kind = getFileKind(mimeType, filename);
  const [failed, setFailed] = useState(false);

  if (failed) {
    return (
      <div className="text-center py-6">
        <p className="mb-2 font-medium">File preview not supported.</p>
        <p className="mb-4 text-sm text-muted-foreground">
          Please download and view it locally.
        </p>
        <div className="flex justify-center gap-2">
          <Button
            variant="secondary"
            onClick={() => window.open(url, "_blank", "noopener,noreferrer")}
          >
            Download
          </Button>
        </div>
      </div>
    );
  }

  switch (kind) {
    case "image":
      return (
        <div className="w-full flex justify-center">
          <img
            src={url}
            alt={filename}
            className="max-w-full max-h-[70vh] object-contain"
            onError={() => setFailed(true)}
          />
        </div>
      );

    case "video":
      return (
        <div className="w-full">
          <video
            controls
            className="w-full max-h-[70vh] bg-black"
            onError={() => setFailed(true)}
          >
            <source src={url} />
            Your browser does not support the video tag.
          </video>
        </div>
      );

    case "audio":
      return (
        <div className="w-full">
          <audio controls className="w-full" onError={() => setFailed(true)}>
            <source src={url} />
            Your browser does not support the audio element.
          </audio>
        </div>
      );

    case "pdf":
      return (
        <div className="w-full">
          <iframe src={url} title={filename} className="w-full h-[80vh]" />
          <div className="mt-2 text-center text-sm text-muted-foreground">
            If the preview appears blank, the file may be blocked by the browser
            or server. You can download it below.
          </div>
          <div className="flex justify-center mt-3">
            <Button
              variant="secondary"
              onClick={() => window.open(url, "_blank", "noopener,noreferrer")}
            >
              Download
            </Button>
          </div>
        </div>
      );

    case "text":
      return (
        <div className="w-full bg-background p-4 rounded border border-drive-border overflow-auto max-h-[70vh]">
          <iframe src={url} title={filename} className="w-full h-[70vh]" />
          <div className="mt-2 text-center text-sm text-muted-foreground">
            If the preview appears blank, try downloading the file to view it
            locally.
          </div>
          <div className="flex justify-center mt-3">
            <Button
              variant="secondary"
              onClick={() => window.open(url, "_blank", "noopener,noreferrer")}
            >
              Download
            </Button>
          </div>
        </div>
      );

    default:
      return (
        <div className="text-center py-6">
          <p className="mb-2 font-medium">File preview not supported.</p>
          <p className="mb-4 text-sm text-muted-foreground">
            Please download and view it locally.
          </p>
          <div className="flex justify-center gap-2">
            <Button
              variant="secondary"
              onClick={() => window.open(url, "_blank", "noopener,noreferrer")}
            >
              Download
            </Button>
          </div>
        </div>
      );
  }
}
