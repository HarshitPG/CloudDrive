import React, { useEffect, useState } from "react";
import { X, FileText, MessageSquare, Eye } from "lucide-react";
import FilePreviewer from "@/components/previewer/FilePreviewer";
import { SummaryPanel, ChatPanel } from "@/components/llm";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { getFileKind } from "@/lib/fileKind";

export type FullscreenPreviewModalProps = {
  open: boolean;
  onClose: () => void;
  url?: string;
  filename?: string;
  mimeType?: string;
  fileId?: string; // Added for LLM features
};

export default function FullscreenPreviewModal({
  open,
  onClose,
  url,
  filename,
  mimeType,
  fileId,
}: FullscreenPreviewModalProps) {
  const [activeTab, setActiveTab] = useState("preview");

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  // Reset to preview tab when modal opens
  useEffect(() => {
    if (open) {
      setActiveTab("preview");
    }
  }, [open]);

  if (!open) return null;

  const fileKind = getFileKind(mimeType, filename);
  const isSupported = fileKind !== "other";
  const isPDF = fileKind === "pdf";

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
        <div className="max-w-7xl mx-auto">
          {filename && (
            <div className="mb-4 text-center text-sm opacity-80 truncate">
              {filename}
            </div>
          )}

          {/* Show tabs only if we have fileId and it's a PDF */}
          {url && fileId && isPDF ? (
            <div className="w-full">
              {/* Small screens: 3 tabs (Preview, Summary, Chat) */}
              <div className="lg:hidden">
                <Tabs
                  value={activeTab}
                  onValueChange={setActiveTab}
                  className="w-full"
                >
                  <TabsList className="grid w-full max-w-md mx-auto grid-cols-3 mb-4">
                    <TabsTrigger
                      value="preview"
                      className="flex items-center gap-2"
                    >
                      <Eye className="w-4 h-4" />
                      <span className="hidden sm:inline">Preview</span>
                    </TabsTrigger>
                    <TabsTrigger
                      value="summary"
                      className="flex items-center gap-2"
                    >
                      <FileText className="w-4 h-4" />
                      <span className="hidden sm:inline">Summary</span>
                    </TabsTrigger>
                    <TabsTrigger
                      value="chat"
                      className="flex items-center gap-2"
                    >
                      <MessageSquare className="w-4 h-4" />
                      <span className="hidden sm:inline">Chat</span>
                    </TabsTrigger>
                  </TabsList>

                  <TabsContent value="preview" className="mt-0">
                    <div className={isSupported ? "" : "text-white"}>
                      <FilePreviewer
                        url={url}
                        filename={filename || "file"}
                        mimeType={mimeType}
                      />
                    </div>
                  </TabsContent>

                  <TabsContent value="summary" className="mt-0">
                    <div className="max-w-4xl mx-auto">
                      <SummaryPanel fileId={fileId} />
                    </div>
                  </TabsContent>

                  <TabsContent value="chat" className="mt-0">
                    <div className="max-w-5xl mx-auto h-[70vh] min-h-[500px]">
                      <ChatPanel fileId={fileId} className="h-full" />
                    </div>
                  </TabsContent>
                </Tabs>
              </div>

              {/* Large screens: top-level tabs for Preview/Chat; Preview shows Summary on the side */}
              <div className="hidden lg:block">
                <Tabs
                  value={activeTab}
                  onValueChange={setActiveTab}
                  className="w-full"
                >
                  <TabsList className="mb-4">
                    <TabsTrigger
                      value="preview"
                      className="flex items-center gap-2"
                    >
                      <Eye className="w-4 h-4" />
                      <span>Preview</span>
                    </TabsTrigger>
                    <TabsTrigger
                      value="chat"
                      className="flex items-center gap-2"
                    >
                      <MessageSquare className="w-4 h-4" />
                      <span>Chat</span>
                    </TabsTrigger>
                  </TabsList>

                  {/* Preview + Summary side-by-side (75% / 25%) */}
                  <TabsContent value="preview" className="mt-0">
                    <div className="grid grid-cols-12 gap-4">
                      <div className="col-span-9">
                        <div className={isSupported ? "" : "text-white"}>
                          <FilePreviewer
                            url={url}
                            filename={filename || "file"}
                            mimeType={mimeType}
                          />
                        </div>
                      </div>
                      <div className="col-span-3">
                        <SummaryPanel fileId={fileId} />
                      </div>
                    </div>
                  </TabsContent>

                  {/* Chat full-height area */}
                  <TabsContent value="chat" className="mt-0">
                    <div className="h-[70vh] min-h-[500px]">
                      <ChatPanel fileId={fileId} className="h-full" />
                    </div>
                  </TabsContent>
                </Tabs>
              </div>
            </div>
          ) : url ? (
            // Fallback to simple preview if no fileId or not PDF
            <div className={isSupported ? "" : "text-white"}>
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
