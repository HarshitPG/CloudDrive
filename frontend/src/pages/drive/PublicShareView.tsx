import React, { useEffect, useState, useCallback } from "react";
import { useParams, Navigate, useSearchParams } from "react-router-dom";
import { motion } from "framer-motion";
import { Button } from "@/components/ui/button";
import { downloadUrlToFile } from "@/lib/utils";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Download,
  FileText,
  Music,
  Video,
  Image as ImageIcon,
  FileArchive,
  Folder,
  ArrowLeft,
  Home,
  ChevronRight,
  Archive,
  Loader2,
  Eye,
} from "lucide-react";
import {
  publicShareApi,
  type SharedFolderData,
  type SharedFolderItem,
} from "@/api/operations";
import FullscreenPreviewModal from "@/components/FullscreenPreviewModal";

const getFileType = (filename: string): string => {
  const ext = filename.toLowerCase().split(".").pop() || "";

  if (["jpg", "jpeg", "png", "gif", "webp", "svg", "bmp"].includes(ext))
    return "image";
  if (["mp4", "avi", "mov", "wmv", "flv", "webm", "mkv"].includes(ext))
    return "video";
  if (["mp3", "wav", "ogg", "flac", "aac", "m4a"].includes(ext)) return "audio";
  if (["pdf"].includes(ext)) return "pdf";
  if (["txt", "md", "json", "xml", "csv", "log"].includes(ext)) return "text";
  if (["doc", "docx", "ppt", "pptx", "xls", "xlsx"].includes(ext))
    return "office";
  if (["zip", "rar", "7z", "tar", "gz"].includes(ext)) return "archive";
  return "unknown";
};

const getFileIcon = (filename: string) => {
  const fileType = getFileType(filename);

  switch (fileType) {
    case "image":
      return ImageIcon;
    case "video":
      return Video;
    case "audio":
      return Music;
    case "archive":
      return Archive;
    default:
      return FileText;
  }
};

const formatBytes = (bytes?: number) => {
  if (!bytes) return "-";
  const sizes = ["Bytes", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return Math.round((bytes / Math.pow(1024, i)) * 100) / 100 + " " + sizes[i];
};

const formatDate = (dateString?: string) => {
  if (!dateString) return "-";
  return new Date(dateString).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
};

// Breadcrumb component for navigation
const Breadcrumb: React.FC<{
  breadcrumbPath: { id: string; name: string }[];
  rootFolderName: string;
  onNavigate: (folderId?: string) => void;
}> = ({ breadcrumbPath, rootFolderName, onNavigate }) => {
  return (
    <div className="flex items-center gap-1 text-sm text-muted-foreground mb-4">
      <Button
        variant="ghost"
        size="sm"
        onClick={() => onNavigate()}
        className="h-auto p-1 text-muted-foreground hover:text-foreground"
      >
        <Home className="w-4 h-4 mr-1" />
        {rootFolderName}
      </Button>
      {breadcrumbPath.map((segment, index) => (
        <React.Fragment key={segment.id}>
          <ChevronRight className="w-4 h-4" />
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onNavigate(segment.id)}
            className="h-auto p-1 text-muted-foreground hover:text-foreground"
          >
            {segment.name}
          </Button>
        </React.Fragment>
      ))}
    </div>
  );
};

const ItemCard: React.FC<{
  item: SharedFolderItem;
  token: string;
  onFolderClick: (folderId: string, folderName: string) => void;
  onPreview: (file: {
    id: string;
    name: string;
    downloadUrl?: string;
    mimeType?: string;
  }) => void;
}> = ({ item, token, onFolderClick, onPreview }) => {
  const [isDownloading, setIsDownloading] = useState(false);

  const IconComponent =
    item.type === "folder" ? Folder : getFileIcon(item.name);

  const handleDownload = async () => {
    if (item.type === "file" && item.downloadUrl) {
      try {
        setIsDownloading(true);
        await downloadUrlToFile(item.downloadUrl, item.name);
      } catch (err) {
        console.error("Programmatic download failed, falling back:", err);
        window.open(item.downloadUrl, "_blank", "noopener,noreferrer");
      } finally {
        setIsDownloading(false);
      }
    } else if (item.type === "file") {
      try {
        setIsDownloading(true);
        const downloadUrl = await publicShareApi.downloadFromShare(
          token,
          item.id
        );
        await downloadUrlToFile(downloadUrl, item.name);
      } catch (err) {
        console.error("Failed to download file:", err);
      } finally {
        setIsDownloading(false);
      }
    }
  };

  return (
    <Card
      className="drive-card cursor-pointer transition-all duration-200"
      onClick={() => {
        if (item.type === "folder") {
          onFolderClick(item.id, item.name);
        }
      }}
    >
      <CardContent className="p-4">
        <div className="flex items-start justify-between mb-3">
          <div
            className={`w-10 h-10 rounded-lg flex items-center justify-center ${
              item.type === "folder"
                ? "bg-primary/10 text-primary"
                : "bg-muted text-muted-foreground"
            }`}
          >
            {isDownloading ? (
              <Loader2 className="w-5 h-5 animate-spin" />
            ) : (
              <IconComponent className="w-5 h-5" />
            )}
          </div>
          {item.type === "file" && (
            <div className="flex gap-1">
              <Button
                variant="ghost"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation();
                  onPreview({
                    id: item.id,
                    name: item.name,
                    downloadUrl: item.downloadUrl,
                    mimeType: item.mimeType,
                  });
                }}
                className="h-8 w-8 p-0"
                title="Preview"
              >
                <Eye className="w-4 h-4" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation();
                  handleDownload();
                }}
                disabled={isDownloading}
                className="h-8 w-8 p-0"
                title="Download"
              >
                <Download className="w-4 h-4" />
              </Button>
            </div>
          )}
        </div>

        <h3 className="font-medium text-foreground text-sm mb-2 truncate">
          {item.name}
        </h3>

        <div className="space-y-2">
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              {item.type === "folder" ? "Folder" : getFileType(item.name)}
            </span>
            <span>{formatBytes(item.size)}</span>
          </div>

          <div className="flex items-center gap-2">
            <Badge variant="secondary" className="text-xs">
              {item.mimeType || "Unknown"}
            </Badge>
          </div>
        </div>
      </CardContent>
    </Card>
  );
};

export default function PublicShareView() {
  const { token } = useParams<{ token: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<SharedFolderData | null>(null);
  const [allItems, setAllItems] = useState<SharedFolderItem[]>([]);
  const [currentItems, setCurrentItems] = useState<SharedFolderItem[]>([]);
  const [breadcrumbPath, setBreadcrumbPath] = useState<
    { id: string; name: string }[]
  >([]);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewData, setPreviewData] = useState<{
    url?: string;
    filename?: string;
    mimeType?: string;
  } | null>(null);

  const currentFolderId = searchParams.get("folderId");

  const buildBreadcrumbPath = useCallback(
    (
      targetFolderId: string,
      items: SharedFolderItem[],
      rootFolderId: string
    ) => {
      const path: { id: string; name: string }[] = [];
      let currentId = targetFolderId;

      while (currentId && currentId !== rootFolderId) {
        const folder = items.find(
          (item) => item.id === currentId && item.type === "folder"
        );
        if (folder) {
          path.unshift({ id: folder.id, name: folder.name });
          currentId = folder.parentId || "";
        } else {
          break;
        }
      }

      setBreadcrumbPath(path);
    },
    []
  );

  const fetchFolderData = useCallback(
    async (folderId?: string) => {
      if (!token) return;

      setLoading(true);
      setError(null);

      try {
        const result = await publicShareApi.resolveFolderShare(token, folderId);
        setData(result);
        setAllItems(result.items);

        const filteredItems = result.items.filter((item) => {
          if (!folderId) {
            return item.parentId === result.share.folderId;
          } else {
            return item.parentId === folderId;
          }
        });

        setCurrentItems(filteredItems);

        if (!folderId) {
          setBreadcrumbPath([]);
        } else {
          buildBreadcrumbPath(folderId, result.items, result.share.folderId);
        }
      } catch (err: unknown) {
        const message =
          (err as { message?: string })?.message ||
          "Failed to load shared folder";
        setError(message);
      } finally {
        setLoading(false);
      }
    },
    [token, buildBreadcrumbPath]
  );

  useEffect(() => {
    fetchFolderData(currentFolderId || undefined);
  }, [fetchFolderData, currentFolderId]);

  const handleNavigateToFolder = (folderId: string, folderName: string) => {
    const newParams = new URLSearchParams(searchParams);
    newParams.set("folderId", folderId);
    setSearchParams(newParams);
  };

  const handleBreadcrumbNavigation = (folderId?: string) => {
    const newParams = new URLSearchParams(searchParams);
    if (folderId) {
      newParams.set("folderId", folderId);
    } else {
      newParams.delete("folderId");
    }
    setSearchParams(newParams);
  };

  const handlePreview = async (file: {
    id: string;
    name: string;
    downloadUrl?: string;
    mimeType?: string;
  }) => {
    try {
      let url = file.downloadUrl;
      if (!url && token) {
        url = await publicShareApi.downloadFromShare(token, file.id);
      }
      setPreviewData({ url, filename: file.name, mimeType: file.mimeType });
      setPreviewOpen(true);
    } catch (e) {
      console.error("Failed to get preview URL", e);
    }
  };

  if (!token) return <Navigate to="/" replace />;

  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25 }}
      className="max-w-7xl mx-auto p-6"
    >
      {loading ? (
        <div className="text-center py-12">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-foreground mx-auto"></div>
          <p className="mt-4 text-muted-foreground">Loading shared folder...</p>
        </div>
      ) : error ? (
        <div className="text-center py-12">
          <h2 className="text-xl font-semibold mb-2">{error}</h2>
          <p className="text-muted-foreground">Unable to open shared folder.</p>
          <Button
            onClick={() => fetchFolderData(currentFolderId || undefined)}
            className="mt-4"
            variant="outline"
          >
            Try Again
          </Button>
        </div>
      ) : data ? (
        <div className="space-y-6">
          {/* Header with folder info */}
          <div className="flex items-center gap-4 p-4 bg-background border border-drive-border rounded-lg">
            <div className="flex-1">
              <h1 className="text-2xl font-semibold flex items-center gap-2">
                <Folder className="w-6 h-6 text-primary" />
                {data.share.title || data.share.folderName || "Shared Folder"}
              </h1>
              {data.share.description && (
                <p className="text-muted-foreground mt-1">
                  {data.share.description}
                </p>
              )}
              <div className="flex gap-2 mt-2">
                {data.share.snapshotMode && (
                  <Badge variant="outline" className="text-xs">
                    Snapshot mode
                  </Badge>
                )}
                {/* Download folder button */}
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={async () => {
                    try {
                      const blob = await publicShareApi.downloadFolderArchive(
                        token!,
                        (downloaded, total) => {
                          console.debug("download progress", downloaded, total);
                        }
                      );
                      const filename = `${
                        data.share.folderName || "shared"
                      }.zip`;
                      const url = URL.createObjectURL(blob);
                      const a = document.createElement("a");
                      a.href = url;
                      a.download = filename;
                      document.body.appendChild(a);
                      a.click();
                      a.remove();
                      URL.revokeObjectURL(url);
                    } catch (err) {
                      console.error("Folder download failed", err);
                      window.open(`/api/v1/fs/${token}/download`, "_blank");
                    }
                  }}
                >
                  <Download className="w-4 h-4 mr-2" />
                  Download Folder
                </Button>
              </div>
            </div>
          </div>

          {/* Breadcrumb navigation */}
          <Breadcrumb
            breadcrumbPath={breadcrumbPath}
            rootFolderName={data.share.folderName || "Shared Folder"}
            onNavigate={handleBreadcrumbNavigation}
          />

          {/* Back button for navigation */}
          {currentFolderId && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                if (breadcrumbPath.length > 1) {
                  const parentFolderId =
                    breadcrumbPath[breadcrumbPath.length - 2]?.id;
                  handleBreadcrumbNavigation(parentFolderId);
                } else {
                  handleBreadcrumbNavigation();
                }
              }}
              className="mb-4"
            >
              <ArrowLeft className="w-4 h-4 mr-2" />
              Back
            </Button>
          )}

          {/* Items grid with Dashboard styling */}
          <div className="space-y-4">
            {/* Items count */}
            {currentItems && currentItems.length > 0 && (
              <div className="flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  {currentItems.length} item
                  {currentItems.length !== 1 ? "s" : ""}(
                  {currentItems.filter((item) => item.type === "folder").length}{" "}
                  folder
                  {currentItems.filter((item) => item.type === "folder")
                    .length !== 1
                    ? "s"
                    : ""}
                  , {currentItems.filter((item) => item.type === "file").length}{" "}
                  file
                  {currentItems.filter((item) => item.type === "file")
                    .length !== 1
                    ? "s"
                    : ""}
                  )
                </p>
              </div>
            )}

            {currentItems && currentItems.length > 0 ? (
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                {currentItems.map((item) => (
                  <ItemCard
                    key={item.id}
                    item={item}
                    token={token}
                    onFolderClick={handleNavigateToFolder}
                    onPreview={handlePreview}
                  />
                ))}
              </div>
            ) : (
              <div className="text-center py-12">
                <Folder className="w-12 h-12 mx-auto mb-4 text-muted-foreground opacity-50" />
                <h3 className="text-lg font-medium mb-2">
                  This folder is empty
                </h3>
                <p className="text-muted-foreground">
                  No files or folders to display
                </p>
              </div>
            )}
          </div>
        </div>
      ) : null}
      <FullscreenPreviewModal
        open={previewOpen}
        onClose={() => setPreviewOpen(false)}
        url={previewData?.url}
        filename={previewData?.filename}
        mimeType={previewData?.mimeType}
      />
    </motion.div>
  );
}
