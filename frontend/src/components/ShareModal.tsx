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

  const [userShareData, setUserShareData] = useState<ShareToUserRequest>({
    targetUserId: "",
    permission: "read",
  });

  const handlePublicShare = async () => {
    setIsLoading(true);
    setError(null);

    try {
      const shareApi =
        item.type === "file"
          ? fileOperationsApi.createPublicFileShare
          : folderOperationsApi.createPublicFolderShare;

      const result = await shareApi(item.id, publicShareData);
      const fullUrl = `${window.location.origin}${result.url}`;

      setGeneratedShareUrl(fullUrl);
      onShareSuccess?.(fullUrl);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create share");
    } finally {
      setIsLoading(false);
    }
  };

  const handleUserShare = async () => {
    if (!userShareData.targetUserId.trim()) {
      setError("Please enter a user ID");
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      if (item.type === "file") {
        await fileOperationsApi.shareFileWithUser(item.id, userShareData);
      } else {
        throw new Error("User sharing for folders not implemented yet");
      }

      setUserShareData({ targetUserId: "", permission: "read" });
      onShareSuccess?.("Shared successfully with user");
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
    setUserShareData({ targetUserId: "", permission: "read" });
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
        <div className="space-y-4">
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
                <Label htmlFor="share-title">Share Title (Optional)</Label>
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
                <Label htmlFor="share-description">
                  Description (Optional)
                </Label>
                <Textarea
                  id="share-description"
                  value={publicShareData.description || ""}
                  onChange={(e) =>
                    setPublicShareData({
                      ...publicShareData,
                      description: e.target.value,
                    })
                  }
                  placeholder="Add a description for this share..."
                  rows={3}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="expiry-date">Expiry Date (Optional)</Label>
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
                <p className="text-xs text-muted-foreground">
                  Leave empty for permanent share
                </p>
              </div>

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
                <Label htmlFor="target-user">User ID</Label>
                <Input
                  id="target-user"
                  value={userShareData.targetUserId}
                  onChange={(e) =>
                    setUserShareData({
                      ...userShareData,
                      targetUserId: e.target.value,
                    })
                  }
                  placeholder="Enter user ID to share with"
                />
                <p className="text-xs text-muted-foreground">
                  The user ID of the person you want to share with
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="permission">Permission Level</Label>
                <select
                  id="permission"
                  value={userShareData.permission}
                  onChange={(e) =>
                    setUserShareData({
                      ...userShareData,
                      permission: e.target.value,
                    })
                  }
                  className="w-full px-3 py-2 bg-background border border-input rounded-md text-sm"
                >
                  <option value="read">Read Only</option>
                  <option value="write">Read & Write</option>
                </select>
              </div>

              <Button
                onClick={handleUserShare}
                disabled={isLoading || !userShareData.targetUserId.trim()}
                className="w-full"
              >
                {isLoading && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                Share with User
              </Button>
            </motion.div>
          )}
        </div>

        <div className="flex justify-end gap-2 pt-4">
          <Button variant="outline" onClick={handleClose}>
            Close
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
