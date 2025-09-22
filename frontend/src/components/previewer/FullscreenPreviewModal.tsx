import React, { useEffect } from "react";
import { X } from "lucide-react";
import FilePreviewer from "@/components/previewer/FilePreviewer";
import { getFileKind } from "@/lib/fileKind";

export type FullscreenPreviewModalProps = {
  open: boolean;
  onClose: () => void;
  url?: string;
  filename?: string;
  mimeType?: string;
};

export default function FullscreenPreviewModal({
  open,
  onClose,
  url,
  filename,
  mimeType,
}: FullscreenPreviewModalProps) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  const fileKind = getFileKind(mimeType, filename);
  const isSupported = fileKind !== "other";

  return (
    <div
      className="fixed inset-0 z-50 bg-black/80 text-white"
      role="dialog"
      aria-modal="true"
      onClick={onClose}
    >
      <button
        type="button"
        aria-label="Close preview"
        className="fixed top-4 right-4 p-2 rounded-md bg-black/60 hover:bg-black/80 z-[9999] pointer-events-auto"
        onClick={onClose}
      >
        <X className="w-5 h-5" />
      </button>
      <div
        className="absolute inset-0 overflow-auto p-4 md:p-8 z-10"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="max-w-6xl mx-auto">
          {filename && (
            <div className="mb-4 text-center text-sm opacity-80 truncate">
              {filename}
            </div>
          )}
          {url ? (
            <div className={isSupported ? " " : "text-white"}>
              <FilePreviewer
                url={url}
                filename={filename || "file"}
                mimeType={mimeType}
              />
            </div>
          ) : (
            <div className="text-center py-10 text-white">
              No preview available.
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
