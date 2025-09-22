import { useEffect, useState, useRef } from "react";
import { uploadManager, type UploadItem } from "@/lib/uploadManager";
import { Button } from "@/components/ui/button";
import { CircleX, X } from "lucide-react";

function ProgressBar({ value }: { value: number }) {
  return (
    <div className="w-full h-2 bg-muted rounded overflow-hidden">
      <div
        className="h-full bg-primary transition-all"
        style={{ width: `${Math.min(100, Math.max(0, value))}%` }}
      />
    </div>
  );
}

export function UploadList() {
  const [items, setItems] = useState<UploadItem[]>(uploadManager.getItems());
  const [open, setOpen] = useState(true);
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    const listener = (i: UploadItem[]) => setItems(i);
    uploadManager.on(listener);
    return () => uploadManager.off(listener);
  }, []);

  useEffect(() => {
    if (timerRef.current) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }

    if (!items.length) return;

    const hasTerminal = items.some((it) =>
      ["done", "error", "aborted"].includes(it.status)
    );
    if (hasTerminal) {
      timerRef.current = window.setTimeout(() => setOpen(false), 3000);
    }

    return () => {
      if (timerRef.current) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [items]);

  if (!open || !items.length) return null;

  return (
    <div className="fixed bottom-4 right-4 w-[min(90vw,420px)] z-50">
      <div className="bg-white border border-drive-border rounded-lg shadow-lg p-3 space-y-3">
        <div className="flex items-center justify-between">
          <div className="text-sm font-medium">Uploads</div>
          <button
            aria-label="Close uploads"
            className="p-1 hover:opacity-80"
            onClick={() => setOpen(false)}
          >
            <CircleX className="w-6 h-6" />
          </button>
        </div>
        <div className="space-y-2 max-h-72 overflow-auto pr-1">
          {items.slice(0, 5).map((it) => (
            <div key={it.id} className="flex items-center gap-3">
              <div className="flex-1 min-w-0">
                <div className="flex items-center justify-between text-xs mb-1">
                  <div className="truncate">{it.file.name}</div>
                  <div className="text-muted-foreground">
                    {Math.round(it.progress)}%
                  </div>
                </div>
                <ProgressBar value={it.progress} />
                <div className="text-[11px] text-muted-foreground mt-1">
                  {it.status === "uploading" && it.speedBps
                    ? `${(it.speedBps / 1024).toFixed(1)} KB/s`
                    : it.status}
                  {it.error ? ` • ${it.error}` : ""}
                </div>
              </div>
              <div className="flex items-center gap-2">
                {it.status === "error" ? (
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => uploadManager.retry(it.id)}
                  >
                    Retry
                  </Button>
                ) : it.status === "uploading" ||
                  it.status === "hashing" ||
                  it.status === "creating" ||
                  it.status === "completing" ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => uploadManager.cancel(it.id)}
                  >
                    Cancel
                  </Button>
                ) : null}
                <button
                  aria-label="Remove"
                  className="p-1 hover:opacity-80"
                  onClick={() => uploadManager.remove(it.id)}
                >
                  <X className="w-3 h-3" />
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
