import { useState, useEffect } from "react";
import { Edit3 } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { renameFolder } from "@/api/folders";
import type { FolderItem } from "@/api/folders";

type FolderEditDialogProps = {
  folder: FolderItem | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onFolderRenamed?: (folderId: string, newName: string) => void;
};

export default function FolderEditDialog({
  folder,
  open,
  onOpenChange,
  onFolderRenamed,
}: FolderEditDialogProps) {
  const [folderName, setFolderName] = useState("");
  const [isRenaming, setIsRenaming] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (folder && open) {
      setFolderName(folder.name);
      setError(null);
    }
  }, [folder, open]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!folder || !folderName.trim()) {
      setError("Folder name is required");
      return;
    }

    if (folderName.trim() === folder.name) {
      onOpenChange(false);
      return;
    }

    setIsRenaming(true);
    setError(null);

    try {
      await renameFolder(folder.id, folderName.trim());
      onFolderRenamed?.(folder.id, folderName.trim());
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to rename folder");
    } finally {
      setIsRenaming(false);
    }
  };

  const handleOpenChange = (newOpen: boolean) => {
    onOpenChange(newOpen);
    if (!newOpen) {
      setFolderName("");
      setError(null);
    }
  };

  if (!folder) return null;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Rename Folder</DialogTitle>
            <DialogDescription>
              Enter a new name for "{folder.name}".
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="folderName" className="text-right">
                Name
              </Label>
              <Input
                id="folderName"
                value={folderName}
                onChange={(e) => setFolderName(e.target.value)}
                placeholder="Folder name"
                className="col-span-3"
                autoFocus
                disabled={isRenaming}
              />
            </div>
            {error && (
              <div className="text-sm text-red-600 bg-red-50 p-2 rounded">
                {error}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isRenaming}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={isRenaming || !folderName.trim()}>
              {isRenaming ? "Renaming..." : "Rename"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
