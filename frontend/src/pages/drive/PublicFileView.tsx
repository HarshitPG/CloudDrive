import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Download, ArrowLeft } from "lucide-react";
import { downloadUrlToFile } from "@/lib/utils";
import FilePreviewer from "@/components/previewer/FilePreviewer";
import type { ResolvedPublicFile } from "@/api/operations";

export default function PublicFileView({ file }: { file: ResolvedPublicFile }) {
  const navigate = useNavigate();
  const [showPreview, setShowPreview] = useState(false);

  return (
    <div className="max-w-4xl mx-auto p-6">
      <div className="mb-4">
        <Button variant="ghost" onClick={() => navigate(-1)}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          Back
        </Button>
      </div>

      <div className="bg-background border border-drive-border rounded-lg p-6">
        <h1 className="text-2xl font-semibold mb-2">{file.filename}</h1>
        <p className="text-sm text-muted-foreground mb-4">
          Size: {file.size ? `${Math.round(file.size / 1024)} KB` : "-"}
        </p>

        <div className="flex gap-2">
          <Button
            variant="secondary"
            onClick={async () => {
              try {
                await downloadUrlToFile(file.download, file.filename);
              } catch (err) {
                window.open(file.download, "_blank", "noopener,noreferrer");
              }
            }}
          >
            <Download className="w-4 h-4 mr-2" />
            Download
          </Button>

          <Button variant="outline" onClick={() => setShowPreview((s) => !s)}>
            {showPreview ? "Hide Preview" : "Preview"}
          </Button>
        </div>
      </div>
      {showPreview && (
        <div className="mt-6">
          <FilePreviewer
            url={file.download}
            filename={file.filename}
            mimeType={
              (file as unknown as Record<string, unknown>).mimeType as
                | string
                | undefined
            }
          />
        </div>
      )}
    </div>
  );
}
