import { useEffect, useMemo, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Folder, Loader2, Search } from "lucide-react";
import { type FolderItem, listFolders } from "@/api/folders";

export type MoveTarget = {
  id: string;
  name: string;
  type: "file" | "folder";
};

interface MoveModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  item: MoveTarget | null;
  onSelectDestination: (destination: FolderItem) => Promise<void> | void;
  disabledFolderId?: string;
}

export default function MoveModal({
  open,
  onOpenChange,
  item,
  onSelectDestination,
  disabledFolderId,
}: MoveModalProps) {
  const [folders, setFolders] = useState<FolderItem[]>([]);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [path, setPath] = useState<Array<{ id?: string; name: string }>>([
    { name: "Home", id: undefined },
  ]);
  const currentParentId = path[path.length - 1]?.id;

  useEffect(() => {
    async function loadFolders() {
      if (!open) return;
      setLoading(true);
      setError(null);
      try {
        const items = await listFolders(currentParentId);
        setFolders(items);
      } catch (e) {
        setError("Failed to load folders");
      } finally {
        setLoading(false);
      }
    }
    loadFolders();
  }, [open, currentParentId]);

  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return folders;
    return folders.filter((f) => f.name.toLowerCase().includes(term));
  }, [folders, search]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            Move {item?.type === "folder" ? "folder" : "file"}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search destination folders..."
              className="pl-10"
            />
          </div>

          {error && (
            <div className="text-sm text-destructive bg-destructive/10 border border-destructive rounded p-2">
              {error}
            </div>
          )}

          <div className="border rounded-md">
            <div className="p-2 border-b flex items-center justify-between gap-2">
              <div className="flex items-center gap-1 text-xs text-muted-foreground flex-wrap">
                {path.map((p, idx) => (
                  <span
                    key={(p.id ?? "root") + idx}
                    className="flex items-center"
                  >
                    <button
                      className={
                        "hover:text-foreground transition-colors " +
                        (idx === path.length - 1
                          ? "font-medium text-foreground"
                          : "")
                      }
                      onClick={() => {
                        setPath((prev) => prev.slice(0, idx + 1));
                      }}
                    >
                      {p.name}
                    </button>
                    {idx < path.length - 1 && (
                      <span className="mx-1 text-muted-foreground">/</span>
                    )}
                  </span>
                ))}
              </div>
              <span className="text-xs text-muted-foreground">
                Select destination folder
              </span>
            </div>
            <div className="max-h-[320px] overflow-y-auto">
              <ul className="divide-y">
                {loading ? (
                  <li className="flex items-center justify-center p-6">
                    <Loader2 className="w-4 h-4 animate-spin" />
                  </li>
                ) : filtered.length === 0 ? (
                  <li className="p-4 text-sm text-muted-foreground">
                    No folders found
                  </li>
                ) : (
                  filtered.map((f) => (
                    <li
                      key={f.id}
                      className="flex items-center justify-between gap-3 p-3 hover:bg-muted/40 transition-colors"
                    >
                      <button
                        className="flex items-center gap-3 min-w-0 flex-1 text-left"
                        onClick={() =>
                          setPath((prev) => [
                            ...prev,
                            { id: f.id, name: f.name },
                          ])
                        }
                        title="Open folder"
                      >
                        <span className="inline-flex items-center justify-center w-8 h-8 rounded-md bg-primary/10 text-primary">
                          <Folder className="w-4 h-4" />
                        </span>
                        <span className="truncate">{f.name}</span>
                      </button>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={disabledFolderId === f.id}
                        onClick={async () => {
                          await onSelectDestination(f);
                          onOpenChange(false);
                        }}
                      >
                        Move here
                      </Button>
                    </li>
                  ))
                )}
              </ul>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
