import { useEffect, useState, useCallback } from "react";
import { motion } from "framer-motion";
import {
  Grid3X3,
  List,
  Search,
  Filter,
  MoreVertical,
  Download,
  Share2,
  Star,
  Trash2,
  Folder,
  FileText,
  Image as ImageIcon,
  Video,
  Music,
  Archive,
  Loader2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import ShareModal from "@/components/ShareModal";
import FilterModal from "@/components/FilterModal";
import { useDriveStore, type DriveItem } from "../../stores/drive";
import { listFiles } from "../../api/files";
import { listFolders } from "../../api/folders";
import {
  searchFiles,
  type SearchFilters,
  type FileSearchResult,
} from "../../api/search";
import { fileOperationsApi, folderOperationsApi } from "../../api/operations";
import { downloadUrlToFile } from "@/lib/utils";
import { UploadList } from "@/components/upload/UploadList";
import { uploadManager } from "@/lib/uploadManager";

const getFileIcon = (mimeType?: string, isFolder?: boolean) => {
  if (isFolder) return Folder;
  if (!mimeType) return FileText;

  if (mimeType.startsWith("image/")) return ImageIcon;
  if (mimeType.startsWith("video/")) return Video;
  if (mimeType.startsWith("audio/")) return Music;
  if (mimeType.includes("zip") || mimeType.includes("archive")) return Archive;

  return FileText;
};

const formatBytes = (bytes?: number) => {
  if (!bytes) return "-";
  const sizes = ["Bytes", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return Math.round((bytes / Math.pow(1024, i)) * 100) / 100 + " " + sizes[i];
};

const formatDate = (dateString: string) => {
  if (!dateString) return "-";
  return new Date(dateString).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
};

export default function Home() {
  const {
    items,
    setItems,
    searchQuery,
    setSearchQuery,
    viewMode,
    setViewMode,
  } = useDriveStore();
  const [filteredItems, setFilteredItems] = useState<DriveItem[]>([]);
  const [searchFilters, setSearchFilters] = useState<SearchFilters>({});
  const [isSearching, setIsSearching] = useState(false);

  const [shareModalOpen, setShareModalOpen] = useState(false);
  const [selectedItem, setSelectedItem] = useState<DriveItem | null>(null);
  const [loadingStates, setLoadingStates] = useState<Record<string, string>>(
    {}
  );
  const [toastMessage, setToastMessage] = useState<string | null>(null);

  const convertSearchResultToDriveItem = useCallback(
    (result: FileSearchResult): DriveItem => {
      return {
        id: result.id,
        name: result.filename,
        filename: result.filename,
        type: "file" as const,
        mimeType: result.mime,
        mime: result.mime,
        size: result.size,
        createdAt: result.createdAt,
        updatedAt: result.updatedAt,
        isShared: false,
        isStarred: false,
        isTrashed: false,
        downloadCount: result.downloadCount,
        tags: [],
        version: 1,
      };
    },
    []
  );

  useEffect(() => {
    // When uploads complete, refresh files/folders in background and merge results silently
    const onDone = async () => {
      try {
        const [filesList, foldersList] = await Promise.all([
          listFiles().catch(() => []),
          listFolders().catch(() => []),
        ]);
        setItems([...foldersList, ...filesList]);
      } catch (e) {
        //
      }
    };
    uploadManager.onDone(onDone);
    return () => {
      uploadManager.offDone(onDone);
    };
  }, [setItems]);

  useEffect(() => {
    let isMounted = true;

    async function loadRoot() {
      try {
        const files = await listFiles().catch((err) => {
          console.error("Failed to load files:", err);
          return [];
        });

        if (!isMounted) return;

        await new Promise((resolve) => setTimeout(resolve, 500));

        const folders = await listFolders().catch((err) => {
          console.error("Failed to load folders:", err);
          return [];
        });

        if (!isMounted) return;
        setItems([...folders, ...files]);
      } catch (error) {
        console.error("Failed to load root:", error);
        if (isMounted) setItems([]);
      }
    }

    const timer = setTimeout(loadRoot, 100);

    return () => {
      isMounted = false;
      clearTimeout(timer);
    };
  }, [setItems]);

  // Search effect
  useEffect(() => {
    // Build search filters including text query
    const filters: SearchFilters = {
      ...searchFilters,
      q: searchQuery.trim() || undefined,
    };

    // If no search query and no other filters, just show regular items
    if (!filters.q && !Object.keys(searchFilters).length) {
      setFilteredItems(items);
      setIsSearching(false);
      return;
    }

    let isMounted = true;
    setIsSearching(true);

    const timer = setTimeout(async () => {
      try {
        const res = await searchFiles(filters);
        if (!isMounted) return;

        // Convert search results to DriveItems
        const driveItems: DriveItem[] = res.items.map(
          convertSearchResultToDriveItem
        );
        setFilteredItems(driveItems);
        setIsSearching(false);
      } catch (error) {
        console.error("Search failed:", error);
        if (isMounted) {
          setFilteredItems([]);
          setIsSearching(false);
        }
      }
    }, 300);

    return () => {
      isMounted = false;
      clearTimeout(timer);
    };
  }, [searchQuery, searchFilters, items, convertSearchResultToDriveItem]);

  // Update filtered items when items change
  useEffect(() => {
    if (!searchQuery.trim()) {
      setFilteredItems(items);
    }
  }, [items, searchQuery]);

  useEffect(() => {
    if (toastMessage) {
      const timer = setTimeout(() => setToastMessage(null), 3000);
      return () => clearTimeout(timer);
    }
  }, [toastMessage]);

  // Operation handlers
  const handleDownload = async (item: DriveItem) => {
    if (item.type === "folder") {
      setToastMessage("Folder download not supported yet");
      return;
    }

    setLoadingStates((prev) => ({ ...prev, [item.id]: "download" }));

    try {
      const result = await fileOperationsApi.downloadFile(item.id);
      // Programmatic download that saves to device rather than navigating
      const filename =
        "filename" in item && item.filename ? item.filename : item.name;
      await downloadUrlToFile(result.downloadUrl, filename);
      setToastMessage("Download completed");
    } catch (error) {
      console.error("Download failed:", error);
      setToastMessage(
        error instanceof Error ? error.message : "Download failed"
      );
    } finally {
      setLoadingStates((prev) => {
        const { [item.id]: _, ...rest } = prev;
        return rest;
      });
    }
  };

  const handleShare = (item: DriveItem) => {
    setSelectedItem(item);
    setShareModalOpen(true);
  };

  const handleDelete = async (item: DriveItem) => {
    if (!confirm(`Are you sure you want to delete "${item.name}"?`)) {
      return;
    }

    setLoadingStates((prev) => ({ ...prev, [item.id]: "delete" }));

    try {
      if (item.type === "file") {
        await fileOperationsApi.deleteFile(item.id);
      } else {
        await folderOperationsApi.deleteFolder(item.id);
      }

      setItems(items.filter((i) => i.id !== item.id));
      setToastMessage(`"${item.name}" moved to trash`);
    } catch (error) {
      console.error("Delete failed:", error);
      setToastMessage(error instanceof Error ? error.message : "Delete failed");
    } finally {
      setLoadingStates((prev) => {
        const { [item.id]: _, ...rest } = prev;
        return rest;
      });
    }
  };

  const handleFiltersChange = (filters: SearchFilters) => {
    setSearchFilters(filters);
  };

  const handleFiltersReset = () => {
    setSearchFilters({});
  };

  const handleShareSuccess = (shareUrl: string) => {
    setToastMessage(
      typeof shareUrl === "string" && shareUrl.startsWith("http")
        ? "Share link copied to clipboard"
        : shareUrl
    );
    setShareModalOpen(false);
  };

  const ItemCard = ({ item }: { item: DriveItem }) => {
    // Create typed metadata object from DriveItem
    const meta = {
      mimeType: "mimeType" in item ? item.mimeType : undefined,
      filename: "filename" in item ? item.filename : undefined,
      size: "size" in item ? item.size : undefined,
      tags: "tags" in item ? item.tags : undefined,
      isStarred: "isStarred" in item ? item.isStarred : undefined,
      isShared: "isShared" in item ? item.isShared : undefined,
      downloadCount: "downloadCount" in item ? item.downloadCount : undefined,
    };

    const IconComponent = getFileIcon(meta.mimeType, item.type === "folder");
    const isLoading = loadingStates[item.id];

    return (
      <motion.div
        initial={{ opacity: 0, scale: 0.95 }}
        animate={{ opacity: 1, scale: 1 }}
        whileHover={{ scale: 1.02 }}
        transition={{ duration: 0.2 }}
      >
        <Card className="drive-card cursor-pointer transition-all duration-200">
          <CardContent className="p-4">
            <div className="flex items-start justify-between mb-3">
              <div
                className={`w-10 h-10 rounded-lg flex items-center justify-center ${
                  item.type === "folder"
                    ? "bg-primary/10 text-primary"
                    : "bg-muted text-muted-foreground"
                }`}
              >
                {isLoading ? (
                  <Loader2 className="w-5 h-5 animate-spin" />
                ) : (
                  <IconComponent className="w-5 h-5" />
                )}
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    className="h-8 w-8 p-0"
                    disabled={!!isLoading}
                  >
                    <MoreVertical className="w-4 h-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onClick={() => handleDownload(item)}
                    disabled={!!isLoading}
                  >
                    <Download className="w-4 h-4 mr-2" />
                    {isLoading === "download" ? "Downloading..." : "Download"}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => handleShare(item)}
                    disabled={!!isLoading}
                  >
                    <Share2 className="w-4 h-4 mr-2" />
                    Share
                  </DropdownMenuItem>
                  <DropdownMenuItem disabled={!!isLoading}>
                    <Star className="w-4 h-4 mr-2" />
                    {meta.isStarred ? "Unstar" : "Star"}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="text-destructive"
                    onClick={() => handleDelete(item)}
                    disabled={!!isLoading}
                  >
                    <Trash2 className="w-4 h-4 mr-2" />
                    {isLoading === "delete" ? "Deleting..." : "Delete"}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>

            <h3 className="font-medium text-foreground text-sm mb-2 truncate">
              {item.name}
            </h3>

            <div className="space-y-2">
              <div className="flex items-center justify-between text-xs text-muted-foreground">
                <span>{formatDate(item.updatedAt || "")}</span>
                <span>{formatBytes(meta.size)}</span>
              </div>

              {meta.tags && meta.tags.length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {meta.tags.slice(0, 2).map((tag: string) => (
                    <Badge key={tag} variant="secondary" className="text-xs">
                      {tag}
                    </Badge>
                  ))}
                  {meta.tags.length > 2 && (
                    <Badge variant="secondary" className="text-xs">
                      +{meta.tags.length - 2}
                    </Badge>
                  )}
                </div>
              )}

              <div className="flex items-center gap-2">
                {meta.isStarred && (
                  <Star className="w-3 h-3 text-warning fill-current" />
                )}
                {meta.isShared && <Share2 className="w-3 h-3 text-primary" />}
                {meta.downloadCount && meta.downloadCount > 0 && (
                  <span className="text-xs text-muted-foreground">
                    {meta.downloadCount} downloads
                  </span>
                )}
              </div>
            </div>
          </CardContent>
        </Card>
      </motion.div>
    );
  };

  return (
    <>
      <div className="flex items-center gap-4 p-4 bg-background border border-drive-border rounded-lg mb-6">
        <div className="flex-1 max-w-md">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
            <Input
              placeholder="Search files and folders..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-10 drive-surface"
            />
            {isSearching && (
              <Loader2 className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 animate-spin text-muted-foreground" />
            )}
          </div>
        </div>

        <div className="flex items-center gap-2 flex-wrap">
          <FilterModal
            filters={{ ...searchFilters, q: searchQuery }}
            onFiltersChange={handleFiltersChange}
            onReset={handleFiltersReset}
          />

          <div className="flex items-center border border-drive-border rounded-lg">
            <Button
              variant={viewMode === "grid" ? "default" : "ghost"}
              size="sm"
              onClick={() => setViewMode("grid")}
              className="border-0 rounded-r-none"
            >
              <Grid3X3 className="w-4 h-4" />
            </Button>
            <Button
              variant={viewMode === "list" ? "default" : "ghost"}
              size="sm"
              onClick={() => setViewMode("list")}
              className="border-0 rounded-l-none"
            >
              <List className="w-4 h-4" />
            </Button>
          </div>
        </div>
      </div>

      <div className="space-y-4">
        {filteredItems.length === 0 ? (
          <div className="text-center py-12">
            <div className="w-16 h-16 bg-muted rounded-full flex items-center justify-center mx-auto mb-4">
              <Folder className="w-8 h-8 text-muted-foreground" />
            </div>
            <h3 className="text-lg font-medium text-foreground mb-2">
              No files found
            </h3>
            <p className="text-muted-foreground">
              {searchQuery
                ? "Try adjusting your search terms"
                : "Upload your first file to get started"}
            </p>
          </div>
        ) : (
          <div
            className={
              viewMode === "grid"
                ? "grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4"
                : "space-y-2"
            }
          >
            {filteredItems.map((item) => (
              <ItemCard key={item.id} item={item} />
            ))}
          </div>
        )}
      </div>

      {selectedItem && (
        <ShareModal
          isOpen={shareModalOpen}
          onClose={() => {
            setShareModalOpen(false);
            setSelectedItem(null);
          }}
          item={{
            id: selectedItem.id,
            name: selectedItem.name,
            type: selectedItem.type,
          }}
          onShareSuccess={handleShareSuccess}
        />
      )}

      {/* Toast Notification */}
      {toastMessage && (
        <motion.div
          initial={{ opacity: 0, y: 50 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 50 }}
          className="fixed bottom-4 right-4 bg-background border border-border rounded-lg shadow-lg p-4 max-w-sm z-50"
        >
          <p className="text-sm">{toastMessage}</p>
        </motion.div>
      )}

      {/* Upload list floating panel */}
      <UploadList />
    </>
  );
}
