import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Search, MoveRight, Folder, Home } from "lucide-react";
import { type FolderItem } from "@/api/folders";
import FolderTreeSelector, { type FolderTreeNode } from "./FolderTreeSelector";

export type MoveTarget = {
  id: string;
  name: string;
  type: "file" | "folder";
};

interface MoveModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  item: MoveTarget | null;
  onSelectDestination: (destination: FolderItem | null) => Promise<void> | void;
  disabledFolderId?: string;
}

export default function MoveModal({
  open,
  onOpenChange,
  item,
  onSelectDestination,
  disabledFolderId,
}: MoveModalProps) {
  const [selectedFolder, setSelectedFolder] = useState<FolderTreeNode | null>(
    null
  );
  const [search, setSearch] = useState("");
  const [moving, setMoving] = useState(false);

  const handleMove = async () => {
    if (!selectedFolder && !item) return;

    setMoving(true);
    try {
      // Convert selectedFolder to FolderItem format or pass null for root
      const destination: FolderItem | null = selectedFolder
        ? {
            id: selectedFolder.id,
            name: selectedFolder.name,
            type: "folder" as const,
            createdAt: selectedFolder.createdAt,
            updatedAt: selectedFolder.updatedAt,
          }
        : null;

      await onSelectDestination(destination);
      onOpenChange(false);
      setSelectedFolder(null);
      setSearch("");
    } catch (error) {
      console.error("Move failed:", error);
    } finally {
      setMoving(false);
    }
  };

  const handleClose = () => {
    onOpenChange(false);
    setSelectedFolder(null);
    setSearch("");
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[95vw] max-w-4xl h-[90vh] max-h-[800px] flex flex-col p-0 gap-0">
        {/* Header */}
        <DialogHeader className="flex-shrink-0 px-8 py-4 border-b  from-background to-muted/30">
          <DialogTitle className="flex items-center gap-3 text-lg md:text-xl">
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <span>Move {item?.type === "folder" ? "folder" : "file"}</span>
                {item && (
                  <span className="font-normal text-muted-foreground text-base truncate">
                    "{item.name}"
                  </span>
                )}
              </div>
              <p className="text-sm text-muted-foreground mt-1">
                Choose a destination folder or select root directory
              </p>
            </div>
          </DialogTitle>
        </DialogHeader>

        {/* Content */}
        <div className="flex-1 flex flex-col min-h-0 p-6 gap-6">
          {/* Search */}
          <div className="flex-shrink-0">
            <label htmlFor="folder-search" className="sr-only">
              Search folders(coming soon)
            </label>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground pointer-events-none" />
              <Input
                id="folder-search"
                value={search}
                disabled
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search folders..."
                className="pl-10 h-11 text-base border-2 focus:border-primary/50 transition-colors"
                autoComplete="off"
              />
            </div>
            {search && (
              <p className="text-xs text-muted-foreground mt-2">
                Searching for "{search}"...
              </p>
            )}
          </div>

          {/* Selected Destination Display */}
          {(selectedFolder || (!selectedFolder && item)) && (
            <div className="flex-shrink-0 p-4 bg-gradient-to-r from-accent/30 via-accent/20 to-accent/10 border border-accent/40 rounded-xl shadow-sm">
              <div className="flex items-center gap-3">
                <div className="flex items-center justify-center w-10 h-10 rounded-full bg-background/80 shadow-sm">
                  {selectedFolder ? (
                    <Folder className="w-5 h-5 text-blue-500" />
                  ) : (
                    <Home className="w-5 h-5 text-gray-600" />
                  )}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                    Moving to
                  </p>
                  <p className="text-sm font-semibold truncate">
                    {selectedFolder ? selectedFolder.name : "Home (Root)"}
                  </p>
                </div>
                <MoveRight className="w-4 h-4 text-muted-foreground flex-shrink-0" />
              </div>
            </div>
          )}

          {/* Folder Tree Container */}
          <div className="flex-1 min-h-0 border-2 border-border/50 rounded-xl bg-background/50 backdrop-blur-sm overflow-hidden">
            <div className="h-full overflow-y-auto">
              <div className="p-4">
                <FolderTreeSelector
                  onSelectFolder={setSelectedFolder}
                  selectedFolderId={selectedFolder?.id}
                  disabledFolderId={disabledFolderId}
                  searchTerm={search}
                  className="h-full"
                />
              </div>
            </div>
          </div>
        </div>

        {/* Footer Actions */}
        <div className="flex-shrink-0 flex flex-col sm:flex-row gap-3 p-6 pt-4 border-t bg-gradient-to-r from-muted/20 to-background">
          <Button
            variant="outline"
            onClick={handleClose}
            disabled={moving}
            className="flex-1 sm:flex-none sm:min-w-[100px] h-11 border-2 hover:border-border transition-colors"
          >
            Cancel
          </Button>
          <Button
            onClick={handleMove}
            disabled={moving}
            className="flex-1 sm:flex-none sm:min-w-[120px] h-11 bg-primary hover:bg-primary/90 shadow-lg hover:shadow-xl transition-all duration-200"
          >
            {moving ? (
              <>
                <div className="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2" />
                Moving...
              </>
            ) : (
              <>
                <MoveRight className="w-4 h-4 mr-2" />
                Move Here
              </>
            )}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
