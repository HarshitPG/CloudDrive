import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { Globe, Users, Copy, Check, Loader2, Share2, X } from "lucide-react";
import {
  fileOperationsApi,
  folderOperationsApi,
  type CreatePublicShareRequest,
  type ShareToUserRequest,
  type FolderShareResponse,
} from "@/api/operations";

export interface ShareModalProps {
  isOpen: boolean;
  onClose: () => void;
  item: {
    id: string;
    name: string;
    type: "file" | "folder";
  };
  onShareSuccess?: (shareUrl: string) => void;
}

export default function ShareModal({
  isOpen,
  onClose,
  item,
  onShareSuccess,
}: ShareModalProps) {
  const [activeTab, setActiveTab] = useState<"public" | "user">("public");
  const [isLoading, setIsLoading] = useState(false);
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [publicShareData, setPublicShareData] =
    useState<CreatePublicShareRequest>({
      title: `Shared ${item.name}`,
      description: "",
      expiresAt: undefined,
    });
  const [generatedShareUrl, setGeneratedShareUrl] = useState<string | null>(
    null
  );

  const [folderShareOptions, setFolderShareOptions] = useState({
    recursive: true,
    snapshotMode: false,
  });

  const [userShareData, setUserShareData] = useState<ShareToUserRequest>(
    () => ({ targetUserEmail: "", permission: "read" })
  );
  const [userShareSuccess, setUserShareSuccess] = useState<string | null>(null);

  const handlePublicShare = async () => {
    setIsLoading(true);
    setError(null);

    try {
      // Normalize expiresAt to full ISO string (RFC3339) or null to match backend *time.Time parsing
      const rawExpires = publicShareData.expiresAt;
      let expiresIso: string | null | undefined = undefined;
      if (rawExpires) {
        try {
          const [datePart, timePart] = rawExpires.split("T");
          if (datePart && timePart) {
            const [year, month, day] = datePart
              .split("-")
              .map((v) => parseInt(v, 10));
            const [hour, minute] = timePart
              .split(":")
              .map((v) => parseInt(v, 10));
            const dt = new Date(year, month - 1, day, hour, minute);
            expiresIso = dt.toISOString();
          }
        } catch (e) {
          const d = new Date(rawExpires as string);
          if (!isNaN(d.getTime())) expiresIso = d.toISOString();
        }
      } else {
        expiresIso = null;
      }

      if (item.type === "file") {
        const payload = {
          title: publicShareData.title,
          description: publicShareData.description,
          expiresAt: expiresIso,
        };
        const result = await fileOperationsApi.createPublicFileShare(
          item.id,
          payload
        );
        const fullUrl = `${window.location.origin}${result.url}`;
        setGeneratedShareUrl(fullUrl);
      } else {
        const payload: CreatePublicShareRequest & {
          recursive?: boolean;
          snapshotMode?: boolean;
        } = {
          title: publicShareData.title,
          description: publicShareData.description,
          expiresAt: expiresIso as string | null | undefined,
          // recursive: folderShareOptions.recursive,
          // snapshotMode: folderShareOptions.snapshotMode,
        };
        const createReq = {
          title: publicShareData.title,
          description: publicShareData.description,
          // recursive: folderShareOptions.recursive,
          // snapshotMode: folderShareOptions.snapshotMode,
          expiresAt: expiresIso as string | null | undefined,
        };
        const result = await folderOperationsApi.createPublicFolderShare(
          item.id,
          createReq
        );
        const fullUrl =
          result.url && result.url.startsWith("http")
            ? result.url
            : `${window.location.origin}${result.url}`;
        setGeneratedShareUrl(fullUrl);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create share");
    } finally {
      setIsLoading(false);
    }
  };

  const handleUserShare = async () => {
    if (!userShareData.targetUserEmail.trim()) {
      setError("Please enter a user email");
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      if (item.type === "file") {
        await fileOperationsApi.shareFileWithUser(item.id, userShareData);
      } else {
        await folderOperationsApi.shareFolderWithUser(item.id, userShareData);
      }

      setUserShareData({ targetUserEmail: "", permission: "read" });
      setUserShareSuccess("User successfully added to share");
      setGeneratedShareUrl(null);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to share with user"
      );
    } finally {
      setIsLoading(false);
    }
  };

  const handleCopyUrl = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      setCopiedUrl(url);
      setTimeout(() => setCopiedUrl(null), 2000);
    } catch (err) {
      setError("Failed to copy URL to clipboard");
    }
  };

  const handleClose = () => {
    setActiveTab("public");
    setPublicShareData({
      title: `Shared ${item.name}`,
      description: "",
      expiresAt: undefined,
    });
    setUserShareData({ targetUserEmail: "", permission: "read" });
    setUserShareSuccess(null);
    setGeneratedShareUrl(null);
    setError(null);
    setCopiedUrl(null);
    onClose();
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Share2 className="w-5 h-5" />
            Share "{item.name}"
          </DialogTitle>
          <DialogDescription>
            Choose how you want to share this {item.type}
          </DialogDescription>
        </DialogHeader>

        {/* Tab Navigation */}
        <div className="flex space-x-1 bg-muted p-1 rounded-lg">
          <button
            onClick={() => setActiveTab("public")}
            className={`flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
              activeTab === "public"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            <Globe className="w-4 h-4" />
            Public Link
          </button>
          <button
            onClick={() => setActiveTab("user")}
            className={`flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
              activeTab === "user"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            <Users className="w-4 h-4" />
            Specific User
          </button>
        </div>

        {/* Error Display */}
        <AnimatePresence>
          {error && (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: "auto" }}
              exit={{ opacity: 0, height: 0 }}
              className="bg-destructive/10 border border-destructive/20 rounded-lg p-3"
            >
              <div className="flex items-center justify-between">
                <p className="text-sm text-destructive">{error}</p>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setError(null)}
                  className="h-auto p-1 text-destructive"
                >
                  <X className="w-4 h-4" />
                </Button>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Tab Content */}
        <div className="space-y-4 max-h-[60vh] overflow-y-auto pr-2">
          {activeTab === "public" && (
            <motion.div
              key="public"
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -20 }}
              transition={{ duration: 0.2 }}
              className="space-y-4"
            >
              <div className="space-y-2">
                <Label htmlFor="share-title">Share Title</Label>
                <Input
                  id="share-title"
                  value={publicShareData.title || ""}
                  onChange={(e) =>
                    setPublicShareData({
                      ...publicShareData,
                      title: e.target.value,
                    })
                  }
                  placeholder={`Shared ${item.name}`}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="share-description">Description</Label>
                <Textarea
                  id="share-description"
                  value={publicShareData.description || ""}
                  onChange={(e) =>
                    setPublicShareData({
                      ...publicShareData,
                      description: e.target.value,
                    })
                  }
                  placeholder="Add an optional description"
                  rows={3}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="expiry-date">Expiry Date</Label>
                <Input
                  id="expiry-date"
                  type="datetime-local"
                  value={publicShareData.expiresAt || ""}
                  onChange={(e) =>
                    setPublicShareData({
                      ...publicShareData,
                      expiresAt: e.target.value,
                    })
                  }
                  min={new Date().toISOString().slice(0, 16)}
                />
              </div>

              {/* Folder-specific options */}
              {item.type === "folder" && (
                <div className="space-y-4 p-4 bg-muted/30 rounded-lg border">
                  <h4 className="text-sm font-medium">Folder Share Options</h4>
                  <p className="text-xs text-muted-foreground mt-1">
                    <b>Note:</b> the current implementation safely shares small
                    folders (a few hundred MB). Multi‑GB folder sharing is
                    disabled because the stream endpoint is fragile (present in
                    local dev: move synchronous ZIP to async archive workflow)
                  </p>

                  <div className="flex items-center justify-between">
                    <div className="space-y-1">
                      <Label htmlFor="recursive">Include Subfolders</Label>
                      <p className="text-xs text-muted-foreground">
                        Share all files and folders inside this folder
                      </p>
                    </div>
                    <input
                      id="recursive"
                      type="checkbox"
                      checked={folderShareOptions.recursive}
                      disabled
                      aria-disabled
                      title="Disabled: large-folder sharing is currently not supported"
                      className="h-4 w-4 rounded border-gray-300 cursor-not-allowed opacity-60"
                    />
                  </div>

                  <div className="flex items-center justify-between">
                    <div className="space-y-1">
                      <Label htmlFor="snapshot">Snapshot Mode</Label>
                      <p className="text-xs text-muted-foreground">
                        Create a fixed snapshot for large folders (better
                        performance)
                      </p>
                    </div>
                    <input
                      id="snapshot"
                      type="checkbox"
                      checked={folderShareOptions.snapshotMode}
                      disabled
                      aria-disabled
                      title="Disabled: snapshot/archive workflow not yet available"
                      className="h-4 w-4 rounded border-gray-300 cursor-not-allowed opacity-60"
                    />
                  </div>
                </div>
              )}

              {generatedShareUrl && (
                <motion.div
                  initial={{ opacity: 0, y: 20 }}
                  animate={{ opacity: 1, y: 0 }}
                  className="space-y-2"
                >
                  <Separator />
                  <Label>Share URL</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      value={generatedShareUrl}
                      readOnly
                      className="font-mono text-sm"
                    />
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => handleCopyUrl(generatedShareUrl)}
                      className="shrink-0"
                    >
                      {copiedUrl === generatedShareUrl ? (
                        <Check className="w-4 h-4" />
                      ) : (
                        <Copy className="w-4 h-4" />
                      )}
                    </Button>
                  </div>
                  <Badge variant="secondary" className="w-fit">
                    Anyone with this link can access
                  </Badge>
                </motion.div>
              )}

              <Button
                onClick={handlePublicShare}
                disabled={isLoading}
                className="w-full"
              >
                {isLoading && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                {generatedShareUrl ? "Generate New Link" : "Create Public Link"}
              </Button>
            </motion.div>
          )}

          {activeTab === "user" && (
            <motion.div
              key="user"
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -20 }}
              transition={{ duration: 0.2 }}
              className="space-y-4"
            >
              <div className="space-y-2">
                <Label htmlFor="target-user">User Email</Label>
                {userShareSuccess && (
                  <div className="bg-emerald-50 border border-emerald-200 rounded-md p-2 text-emerald-800 text-sm">
                    {userShareSuccess}
                  </div>
                )}
                <Input
                  id="target-user"
                  value={userShareData.targetUserEmail}
                  onChange={(e) => {
                    setUserShareData({
                      ...userShareData,
                      targetUserEmail: e.target.value,
                    });
                    setUserShareSuccess(null);
                  }}
                  placeholder="Enter user email to share with"
                />
                <p className="text-xs text-muted-foreground">
                  The email address of the person you want to share with
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="permission">
                  Permission Level (Coming soon)
                </Label>
                <select
                  id="permission"
                  value={userShareData.permission}
                  disabled
                  className="w-full px-3 py-2 bg-background border border-input rounded-md text-sm"
                >
                  <option value="coming-soon">Coming soon</option>
                </select>
              </div>

              <Button
                onClick={handleUserShare}
                disabled={isLoading || !userShareData.targetUserEmail.trim()}
                className="w-full"
              >
                {isLoading && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                Share with User
              </Button>
            </motion.div>
          )}
        </div>

        <div className="flex justify-end ">
          <Button variant="outline" onClick={handleClose}>
            Close
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
