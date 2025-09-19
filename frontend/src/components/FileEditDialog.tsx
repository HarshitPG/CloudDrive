import React, { useState, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import type { FileItem } from "@/api/files";

type Props = {
  open: boolean;
  file?: FileItem | null;
  onClose: () => void;
  onSaved?: (updated: FileItem) => void;
};

export default function FileEditDialog({
  open,
  file,
  onClose,
  onSaved,
}: Props) {
  const [filename, setFilename] = useState("");
  const [tagsStr, setTagsStr] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (file) {
      setFilename(file.filename || file.name || "");
      setTagsStr((file.tags || []).join(", "));
      setError(null);
    } else {
      setFilename("");
      setTagsStr("");
      setError(null);
    }
  }, [file]);

  const handleSave = async () => {
    if (!file) return;
    const newName = filename.trim();
    const tags = tagsStr
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);
    if (!newName) {
      setError("Filename cannot be empty");
      return;
    }
    setIsSaving(true);
    setError(null);
    try {
      const { patchFile } = await import("@/api/files");
      await patchFile(file.id, { filename: newName, tags });
      // Build a local updated snapshot (optimistic + canonical enough for UI)
      const updated = {
        ...file,
        name: newName,
        filename: newName,
        tags,
        updatedAt: new Date().toISOString(),
      } as FileItem;
      onSaved?.(updated);
      onClose();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      setError(message || "Failed to save");
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit file</DialogTitle>
          <DialogDescription>Rename file or edit tags.</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 mt-4">
          <div>
            <label className="text-xs font-medium text-muted-foreground">
              Filename
            </label>
            <Input
              value={filename}
              onChange={(e) => setFilename(e.target.value)}
            />
          </div>
          <div>
            <label className="text-xs font-medium text-muted-foreground">
              Tags (comma separated)
            </label>
            <Input
              value={tagsStr}
              onChange={(e) => setTagsStr(e.target.value)}
            />
          </div>
          {error && <div className="text-sm text-destructive">{error}</div>}
        </div>

        <DialogFooter>
          <div className="flex gap-2">
            <Button variant="ghost" onClick={onClose} disabled={isSaving}>
              Cancel
            </Button>
            <Button onClick={handleSave} disabled={isSaving}>
              {isSaving ? "Saving..." : "Save"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
