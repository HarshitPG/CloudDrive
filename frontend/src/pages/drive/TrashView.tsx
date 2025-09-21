import { motion } from "framer-motion";
import { Trash2, Menu } from "lucide-react";
import { useEffect, useState } from "react";
import {
  listDeletedFiles,
  restoreFile,
  deleteFilePermanent,
} from "../../api/files";
import {
  listDeletedFolders,
  deleteFolderPermanent,
  FolderItem,
} from "../../api/folders";
import { Button } from "../../components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "../../components/ui/dialog";
import { RotateCw, Trash } from "lucide-react";

type DeletedFile = {
  id: string;
  filename: string;
  mime?: string;
  size?: number;
  deletedAt?: string;
};
type DeletedFolder = {
  id: string;
  name: string;
  deletedAt?: string;
};

type BackendFolderResponse = {
  id: string;
  name?: string;
  deletedAt?: string;
  deleted_at?: string;
};
type BackendDeletedFile = {
  id: string;
  filename?: string;
  name?: string;
  mime?: string;
  size?: number;
  deletedAt?: string;
  deleted_at?: string;
};

export default function TrashView() {
  const [files, setFiles] = useState<DeletedFile[]>([]);
  const [folders, setFolders] = useState<DeletedFolder[]>([]);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  const fetchTrash = async () => {
    setLoading(true);
    try {
      const [fileData, folderData] = await Promise.all([
        listDeletedFiles(),
        listDeletedFolders(),
      ]);
      const filesMapped = (fileData || []).map((f: BackendDeletedFile) => ({
        id: String(f.id),
        filename: String(f.filename ?? f.name ?? ""),
        mime: f.mime,
        size: f.size,
        deletedAt: (f.deletedAt ?? f.deleted_at) as string | undefined,
      }));
      const foldersBackend = folderData as BackendFolderResponse[];
      const foldersMapped = (foldersBackend || []).map((f) => ({
        id: f.id,
        name: f.name ?? "",
        deletedAt: f.deletedAt ?? f.deleted_at ?? "",
      }));
      setFiles(filesMapped);
      setFolders(foldersMapped);
    } catch (err) {
      console.error("failed load trash", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTrash();
  }, []);

  const onRestore = async (id: string) => {
    // show confirmation modal handled outside this function
    setActionLoading(id);
    try {
      // determine whether id is file or folder by checking local lists
      const isFolder = folders.find((f) => f.id === id);
      if (isFolder) {
        // no folder restore endpoint implemented yet, call backend when available
        // placeholder: treat as no-op and remove from UI
        setFolders((s) => s.filter((ff) => ff.id !== id));
      } else {
        await restoreFile(id);
        setFiles((s) => s.filter((f) => f.id !== id));
      }
    } catch (err) {
      console.error("restore failed", err);
      // use dialog-based error or toast in future; fallback to alert
      alert("Failed to restore file. See console for details.");
    } finally {
      setActionLoading(null);
    }
  };

  const onDeletePermanent = async (id: string) => {
    setActionLoading(id);
    try {
      const isFolder = folders.find((f) => f.id === id);
      if (isFolder) {
        await deleteFolderPermanent(id);
        setFolders((s) => s.filter((f) => f.id !== id));
      } else {
        await deleteFilePermanent(id);
        setFiles((s) => s.filter((f) => f.id !== id));
      }
    } catch (err) {
      console.error("permanent delete failed", err);
      alert("Failed to delete file permanently. See console for details.");
    } finally {
      setActionLoading(null);
    }
  };

  // dialog state
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogAction, setDialogAction] = useState<{
    type: "restore" | "delete";
    id: string;
    filename: string;
  } | null>(null);

  const openConfirm = (
    type: "restore" | "delete",
    id: string,
    filename: string
  ) => {
    setDialogAction({ type, id, filename });
    setDialogOpen(true);
  };

  const handleConfirm = async () => {
    if (!dialogAction) return;
    const { type, id } = dialogAction;
    setDialogOpen(false);
    if (type === "restore") await onRestore(id);
    else await onDeletePermanent(id);
    setDialogAction(null);
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="space-y-6"
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div>
            <h1 className="text-2xl font-bold text-foreground mb-2">Trash</h1>
            <p className="text-muted-foreground">
              Deleted files (items are restorable)
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost" onClick={fetchTrash} disabled={loading}>
            Refresh
          </Button>
        </div>
      </div>

      {loading ? (
        <div className="text-center py-12">Loading...</div>
      ) : files.length + folders.length === 0 ? (
        <div className="text-center py-12">
          <div className="w-16 h-16 bg-muted rounded-full flex items-center justify-center mx-auto mb-4">
            <Trash2 className="w-8 h-8 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium text-foreground mb-2">
            Trash is empty
          </h3>
          <p className="text-muted-foreground">
            Items you delete will appear here
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          <div className="overflow-x-auto">
            <table className="w-full table-auto text-sm">
              <thead>
                <tr className="text-left text-muted-foreground">
                  <th className="px-3 py-2">Name</th>
                  <th className="px-3 py-2">Type</th>
                  <th className="px-3 py-2">Size</th>
                  <th className="px-3 py-2">Deleted</th>
                  <th className="px-3 py-2">Actions</th>
                </tr>
              </thead>
              <tbody>
                {folders.map((ff) => (
                  <tr key={`folder-${ff.id}`} className="border-t">
                    <td className="px-3 py-2">
                      <div className="font-medium">{ff.name}</div>
                    </td>
                    <td className="px-3 py-2">folder</td>
                    <td className="px-3 py-2">-</td>
                    <td className="px-3 py-2">{ff.deletedAt ?? "-"}</td>
                    <td className="px-3 py-2">
                      <div className="flex items-center gap-2">
                        <Button
                          size="icon"
                          variant="ghost"
                          onClick={() => openConfirm("restore", ff.id, ff.name)}
                          aria-label={`Restore ${ff.name}`}
                          disabled={actionLoading !== null}
                        >
                          <RotateCw className="w-4 h-4" />
                        </Button>
                        <Button
                          size="icon"
                          variant="ghost"
                          onClick={() => openConfirm("delete", ff.id, ff.name)}
                          aria-label={`Delete permanently ${ff.name}`}
                          disabled={actionLoading !== null}
                        >
                          <Trash className="w-4 h-4 text-destructive" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
                {files.map((f) => (
                  <tr key={`file-${f.id}`} className="border-t">
                    <td className="px-3 py-2">
                      <div className="font-medium">{f.filename}</div>
                    </td>
                    <td className="px-3 py-2">{f.mime ?? "-"}</td>
                    <td className="px-3 py-2">{f.size ?? "-"}</td>
                    <td className="px-3 py-2">{f.deletedAt ?? "-"}</td>
                    <td className="px-3 py-2">
                      <div className="flex items-center gap-2">
                        <Button
                          size="icon"
                          variant="ghost"
                          onClick={() =>
                            openConfirm("restore", f.id, f.filename)
                          }
                          aria-label={`Restore ${f.filename}`}
                          disabled={actionLoading !== null}
                        >
                          <RotateCw className="w-4 h-4" />
                        </Button>
                        <Button
                          size="icon"
                          variant="ghost"
                          onClick={() =>
                            openConfirm("delete", f.id, f.filename)
                          }
                          aria-label={`Delete permanently ${f.filename}`}
                          disabled={actionLoading !== null}
                        >
                          <Trash className="w-4 h-4 text-destructive" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
      <TrashConfirmDialog
        open={dialogOpen}
        onOpenChange={(v) => setDialogOpen(v)}
        action={dialogAction}
        onConfirm={handleConfirm}
        loading={actionLoading !== null}
      />
    </motion.div>
  );
}

// Note: Dialog is placed outside the return earlier for easier control via state; we append it here

// Confirmation dialog
export function TrashConfirmDialog(props: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  action: { type: "restore" | "delete"; id: string; filename: string } | null;
  onConfirm: () => Promise<void> | void;
  loading?: boolean;
}) {
  const { open, onOpenChange, action, onConfirm, loading } = props;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {action?.type === "delete"
              ? "Delete permanently?"
              : "Restore file?"}
          </DialogTitle>
          <DialogDescription>
            {action?.type === "delete" ? (
              <>
                This will permanently delete <strong>{action.filename}</strong>.
                This action cannot be undone.
              </>
            ) : (
              <>
                Restore <strong>{action?.filename}</strong> from Trash back to
                its original location.
              </>
            )}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <div className="flex gap-2">
            <Button
              variant="ghost"
              onClick={() => onOpenChange(false)}
              disabled={loading}
            >
              Cancel
            </Button>
            <Button
              variant={action?.type === "delete" ? "destructive" : "secondary"}
              onClick={async () => await onConfirm()}
              disabled={loading}
            >
              {action?.type === "delete" ? "Delete" : "Restore"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// Add the dialog into module default usage by exporting and letting the parent render it via state
