import { useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Upload } from "lucide-react";

export function UploadArea({
  onSelect,
}: {
  onSelect: (files: FileList) => void;
}) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [dragOver, setDragOver] = useState(false);

  return (
    <div
      className={`flex items-center gap-3 border border-drive-border rounded-lg p-3 ${
        dragOver ? "bg-muted" : "bg-background"
      }`}
      onDragOver={(e) => {
        e.preventDefault();
        setDragOver(true);
      }}
      onDragLeave={(e) => {
        e.preventDefault();
        setDragOver(false);
      }}
      onDrop={(e) => {
        e.preventDefault();
        setDragOver(false);
        if (e.dataTransfer?.files && e.dataTransfer.files.length) {
          onSelect(e.dataTransfer.files);
        }
      }}
    >
      <input
        ref={inputRef}
        type="file"
        className="hidden"
        multiple
        onChange={(e) => {
          if (e.target.files && e.target.files.length) {
            onSelect(e.target.files);
            e.currentTarget.value = "";
          }
        }}
      />
      <Button size="sm" onClick={() => inputRef.current?.click()}>
        <Upload className="w-4 h-4 mr-2" /> Upload
      </Button>
      <div className="text-sm text-muted-foreground">
        or drag & drop files here
      </div>
    </div>
  );
}
