import { useEffect, useState, useCallback, type MouseEvent } from "react";
import { useParams, useNavigate } from "react-router-dom";
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
  Edit3,
  ChevronRight,
  Home as HomeIcon,
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
import MoveModal from "@/components/MoveModal";
import FileEditDialog from "@/components/FileEditDialog";
import FolderEditDialog from "@/components/FolderEditDialog";
import CreateFolderDialog from "@/components/CreateFolderDialog";
import FilterModal from "@/components/FilterModal";
import { useDriveStore, type DriveItem } from "../../stores/drive";
import type { FileItem } from "../../api/files";
import type { FolderItem } from "../../api/folders";
import { listFiles } from "../../api/files";
import { listFolders, deleteFolder, getFolderFiles } from "../../api/folders";
import {
  searchFiles,
  type SearchFilters,
  type FileSearchResult,
} from "../../api/search";
import { fileOperationsApi, folderOperationsApi } from "../../api/operations";
import { downloadUrlToFile } from "@/lib/utils";
import { UploadList } from "@/components/upload/UploadList";
import { uploadManager } from "@/lib/uploadManager";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";

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
  const { folderId } = useParams<{ folderId: string }>();
  const navigate = useNavigate();

  const {
    items,
    setItems,
    searchQuery,
    setSearchQuery,
    viewMode,
    setViewMode,
    addItem,
    updateItem,
    removeItem,
    currentFolderId,
    setCurrentFolderId,
  } = useDriveStore();
  const [filteredItems, setFilteredItems] = useState<DriveItem[]>([]);
  const [searchFilters, setSearchFilters] = useState<SearchFilters>({});
  const [isSearching, setIsSearching] = useState(false);

  const [shareModalOpen, setShareModalOpen] = useState(false);
  const [selectedItem, setSelectedItem] = useState<DriveItem | null>(null);
  const [editFile, setEditFile] = useState<FileItem | null>(null);
  const [editOpen, setEditOpen] = useState(false);
  const [editFolder, setEditFolder] = useState<FolderItem | null>(null);
  const [folderEditOpen, setFolderEditOpen] = useState(false);
  const [loadingStates, setLoadingStates] = useState<Record<string, string>>(
    {}
  );
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const [currentFolder, setCurrentFolder] = useState<FolderItem | null>(null);
  const [breadcrumbs, setBreadcrumbs] = useState<
    { id: string; name: string }[]
  >([]);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<DriveItem | null>(null);
  const [moveOpen, setMoveOpen] = useState(false);
  const [moveTarget, setMoveTarget] = useState<DriveItem | null>(null);
  const isInFolder = !!folderId;
  const isSearchDisabled = isInFolder;

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

  const loadData = useCallback(async () => {
    try {
      if (folderId) {
        const [foldersList, filesList] = await Promise.all([
          listFolders(folderId).catch(() => []),
          getFolderFiles(folderId).catch(() => []),
        ]);
        const transformedFiles = filesList.map((file) => ({
          id: file.id,
          name: file.filename,
          filename: file.filename,
          type: "file" as const,
          mimeType: file.mime,
          mime: file.mime,
          size: file.size,
          createdAt: file.createdAt,
          updatedAt: file.updatedAt,
          isShared: false,
          isStarred: false,
          isTrashed: false,
          downloadCount: file.downloadCount,
          tags: [],
          version: 1,
          contentHash: file.contentHash,
          physicalSize: file.physicalSize,
          refCount: file.refCount,
          dedupSavings: file.dedupSavings,
        }));

        setItems([...foldersList, ...transformedFiles]);
        setCurrentFolderId(folderId);
      } else {
        const [filesList, foldersList] = await Promise.all([
          listFiles().catch(() => []),
          listFolders().catch(() => []),
        ]);
        setItems([...foldersList, ...filesList]);
        setCurrentFolderId(undefined);
      }
    } catch (e) {
      console.error("Failed to load data:", e);
    }
  }, [setItems, folderId, setCurrentFolderId]);

  useEffect(() => {
    const onDone = async () => {
      await loadData();
    };
    uploadManager.onDone(onDone);
    return () => {
      uploadManager.offDone(onDone);
    };
  }, [loadData]);

  useEffect(() => {
    if (isSearchDisabled && searchQuery) {
      setSearchQuery("");
    }
  }, [isSearchDisabled, searchQuery, setSearchQuery]);

  useEffect(() => {
    let isMounted = true;

    async function loadRoot() {
      await loadData();
    }

    const timer = setTimeout(loadRoot, 100);

    return () => {
      isMounted = false;
      clearTimeout(timer);
    };
  }, [loadData]);

  // Search effect
  useEffect(() => {
    // Disable search when in folder view
    if (isSearchDisabled) {
      setFilteredItems(items);
      setIsSearching(false);
      return;
    }

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
  }, [
    searchQuery,
    searchFilters,
    items,
    convertSearchResultToDriveItem,
    isSearchDisabled,
  ]);

  // Update filtered items when items change
  useEffect(() => {
    if (!searchQuery.trim() || isSearchDisabled) {
      setFilteredItems(items);
    }
  }, [items, searchQuery, isSearchDisabled]);

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

  const openMoveModal = (item: DriveItem) => {
    setMoveTarget(item);
    setMoveOpen(true);
  };
  const openDeleteModal = (item: DriveItem) => {
    setDeleteTarget(item);
    setDeleteModalOpen(true);
  };

  const performDelete = async (item: DriveItem | null) => {
    if (!item) return;

    setDeleteModalOpen(false);
    setLoadingStates((prev) => ({ ...prev, [item.id]: "delete" }));

    try {
      if (item.type === "file") {
        await fileOperationsApi.deleteFile(item.id);
      } else {
        await deleteFolder(item.id);
      }

      removeItem(item.id);
      setToastMessage(`"${item.name}" moved to trash`);
    } catch (error) {
      console.error("Delete failed:", error);
      setToastMessage(error instanceof Error ? error.message : "Delete failed");
    } finally {
      setLoadingStates((prev) => {
        const { [item.id]: _, ...rest } = prev;
        return rest;
      });
      setDeleteTarget(null);
    }
  };

  const handleFiltersChange = (filters: SearchFilters) => {
    setSearchFilters(filters);
  };

  const handleFiltersReset = () => {
    setSearchFilters({});
  };

  const handleFolderCreated = async (folderName: string) => {
    await loadData();
    setToastMessage(`Folder "${folderName}" created`);
  };

  const handleFolderRenamed = (folderId: string, newName: string) => {
    updateItem(folderId, { name: newName } as Partial<DriveItem>);
    setToastMessage(`Folder renamed to "${newName}"`);
  };

  const handleEditFolder = (folder: FolderItem) => {
    setEditFolder(folder);
    setFolderEditOpen(true);
  };

  const handleFolderClick = (folder: FolderItem) => {
    navigate(`/dashboard/home/folder/${folder.id}`);
  };

  const handleBreadcrumbClick = (folderId?: string) => {
    if (folderId) {
      navigate(`/dashboard/home/folder/${folderId}`);
    } else {
      navigate("/dashboard/home");
    }
  };

  const Breadcrumb = () => {
    if (!isInFolder) return null;

    return (
      <div className="flex items-center gap-2 mb-4">
        <button
          onClick={() => handleBreadcrumbClick()}
          className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <HomeIcon className="w-4 h-4" />
          Home
        </button>
        {folderId && (
          <>
            <ChevronRight className="w-4 h-4 text-muted-foreground" />
            <span className="text-sm font-medium">
              {currentFolder?.name || "Current Folder"}
            </span>
          </>
        )}
      </div>
    );
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
      <div>
        <Card
          className="drive-card cursor-pointer transition-all duration-200"
          onClick={() => {
            if (item.type === "folder") {
              handleFolderClick(item as FolderItem);
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
                    onClick={(e: MouseEvent<HTMLButtonElement>) => {
                      e.stopPropagation();
                    }}
                  >
                    <MoreVertical className="w-4 h-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {item.type === "file" && (
                    <>
                      <DropdownMenuItem
                        onClick={(e: MouseEvent) => {
                          e.stopPropagation();
                          e.preventDefault();
                          handleDownload(item);
                        }}
                        disabled={!!isLoading}
                      >
                        <Download className="w-4 h-4 mr-2" />
                        {isLoading === "download"
                          ? "Downloading..."
                          : "Download"}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={(e: MouseEvent) => {
                          e.stopPropagation();
                          e.preventDefault();
                          handleShare(item);
                        }}
                        disabled={!!isLoading}
                      >
                        <Share2 className="w-4 h-4 mr-2" />
                        Share
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={(e: MouseEvent) => {
                          e.stopPropagation();
                          e.preventDefault();
                          setEditFile(item as FileItem);
                          setEditOpen(true);
                        }}
                        disabled={!!isLoading}
                      >
                        <Edit3 className="w-4 h-4 mr-2" />
                        Edit
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={(e: MouseEvent) => {
                          e.stopPropagation();
                          e.preventDefault();
                          openMoveModal(item);
                        }}
                        disabled={!!isLoading}
                      >
                        <Folder className="w-4 h-4 mr-2" />
                        Move
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        disabled={!!isLoading}
                        onClick={(e: MouseEvent) => {
                          e.stopPropagation();
                          e.preventDefault();
                        }}
                      >
                        <Star className="w-4 h-4 mr-2" />
                        {meta.isStarred ? "Unstar" : "Star"}
                      </DropdownMenuItem>
                    </>
                  )}

                  {item.type === "folder" && (
                    <>
                      <DropdownMenuItem
                        onClick={(e: MouseEvent) => {
                          e.stopPropagation();
                          e.preventDefault();
                          handleEditFolder(item as FolderItem);
                        }}
                        disabled={!!isLoading}
                      >
                        <Edit3 className="w-4 h-4 mr-2" />
                        Rename
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={(e: MouseEvent) => {
                          e.stopPropagation();
                          e.preventDefault();
                          handleShare(item);
                        }}
                        disabled={!!isLoading}
                      >
                        <Share2 className="w-4 h-4 mr-2" />
                        Share
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={(e: MouseEvent) => {
                          e.stopPropagation();
                          e.preventDefault();
                          openMoveModal(item);
                        }}
                        disabled={!!isLoading}
                      >
                        <Folder className="w-4 h-4 mr-2" />
                        Move
                      </DropdownMenuItem>
                    </>
                  )}

                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="text-destructive"
                    onClick={(e: MouseEvent) => {
                      e.stopPropagation();
                      e.preventDefault();
                      openDeleteModal(item);
                    }}
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
      </div>
    );
  };

  return (
    <>
      <Breadcrumb />

      <div className="flex items-center gap-4 p-4 bg-background border border-drive-border rounded-lg mb-6">
        <div className="flex-1 max-w-md">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
            <Input
              placeholder={
                isSearchDisabled
                  ? "Search disabled in folder view"
                  : "Search files and folders..."
              }
              value={searchQuery}
              onChange={(e) =>
                !isSearchDisabled && setSearchQuery(e.target.value)
              }
              className="pl-10 drive-surface"
              disabled={isSearchDisabled}
            />
            {isSearching && !isSearchDisabled && (
              <Loader2 className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 animate-spin text-muted-foreground" />
            )}
          </div>
        </div>

        <div className="flex items-center gap-2 flex-wrap">
          <CreateFolderDialog
            parentFolderId={folderId}
            onFolderCreated={handleFolderCreated}
          />
          {!isSearchDisabled && (
            <FilterModal
              filters={{ ...searchFilters, q: searchQuery }}
              onFiltersChange={handleFiltersChange}
              onReset={handleFiltersReset}
            />
          )}{" "}
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
              {isInFolder ? "Folder is empty" : "No files found"}
            </h3>
            <p className="text-muted-foreground">
              {isInFolder
                ? "This folder doesn't contain any files or subfolders yet"
                : searchQuery
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

      <FileEditDialog
        open={editOpen}
        file={editFile}
        onClose={() => {
          setEditOpen(false);
          setEditFile(null);
        }}
        onSaved={(updated) => {
          const newItems = items.map((i) =>
            i.id === updated.id
              ? ({
                  ...i,
                  name: updated.name,
                  filename: updated.filename,
                  updatedAt: updated.updatedAt,
                  tags: updated.tags,
                } as DriveItem)
              : i
          );
          setItems(newItems);
          setToastMessage("File updated");
        }}
      />

      <FolderEditDialog
        folder={editFolder}
        open={folderEditOpen}
        onOpenChange={setFolderEditOpen}
        onFolderRenamed={handleFolderRenamed}
      />

      {/* Move modal */}
      {moveTarget && (
        <MoveModal
          open={moveOpen}
          onOpenChange={(o) => {
            setMoveOpen(o);
            if (!o) setMoveTarget(null);
          }}
          item={{
            id: moveTarget.id,
            name: moveTarget.name,
            type: moveTarget.type,
          }}
          disabledFolderId={
            moveTarget.type === "folder" ? moveTarget.id : undefined
          }
          onSelectDestination={async (dest) => {
            try {
              const targetId = dest?.id || null;
              const targetName = dest?.name || "Home";

              if (moveTarget.type === "file") {
                await fileOperationsApi.moveFile(moveTarget.id, targetId);
              } else {
                await folderOperationsApi.moveFolder(moveTarget.id, targetId);
              }
              await loadData();
              setToastMessage(`"${moveTarget.name}" moved to "${targetName}"`);
            } catch (e) {
              console.error(e);
              setToastMessage(e instanceof Error ? e.message : "Move failed");
            }
          }}
        />
      )}

      {/* Delete confirmation modal */}
      <Dialog open={deleteModalOpen} onOpenChange={setDeleteModalOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Confirm delete</DialogTitle>
            <DialogDescription>
              {deleteTarget && deleteTarget.type === "folder" ? (
                <>
                  You're about to move the folder "{deleteTarget.name}" to the
                  trash. This will soft-delete the folder and its subtree.
                </>
              ) : (
                <>You're about to move "{deleteTarget?.name}" to the trash.</>
              )}
            </DialogDescription>
          </DialogHeader>
          <div className="mt-4 flex gap-2 justify-end">
            <Button
              variant="outline"
              onClick={() => setDeleteModalOpen(false)}
              disabled={!!(deleteTarget && loadingStates[deleteTarget.id])}
            >
              Cancel
            </Button>
            <Button
              className="text-destructive"
              onClick={() => performDelete(deleteTarget)}
              disabled={!!(deleteTarget && loadingStates[deleteTarget.id])}
            >
              {deleteTarget && loadingStates[deleteTarget.id] === "delete"
                ? "Deleting..."
                : "Delete"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* Toast Notification */}
      {toastMessage && (
        <div className="fixed bottom-4 right-4 bg-background border border-border rounded-lg shadow-lg p-4 max-w-sm z-50">
          <p className="text-sm">{toastMessage}</p>
        </div>
      )}

      {/* Upload list floating panel */}
      <UploadList />
    </>
  );
}
