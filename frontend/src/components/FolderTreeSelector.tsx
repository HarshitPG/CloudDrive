import { useState, useEffect, useCallback, useMemo } from "react";
import {
  ChevronRight,
  ChevronDown,
  Folder,
  FolderOpen,
  Home,
  AlertCircle,
  Loader2,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { type FolderItem, listFolders } from "@/api/folders";
import { Button } from "@/components/ui/button";

export interface FolderTreeNode extends FolderItem {
  children?: FolderTreeNode[];
  expanded?: boolean;
  loading?: boolean;
  level: number;
}

interface FolderTreeSelectorProps {
  onSelectFolder: (folder: FolderTreeNode | null) => void;
  selectedFolderId?: string;
  disabledFolderId?: string;
  className?: string;
  searchTerm?: string;
}

export default function FolderTreeSelector({
  onSelectFolder,
  selectedFolderId,
  disabledFolderId,
  className,
  searchTerm = "",
}: FolderTreeSelectorProps) {
  const [rootFolders, setRootFolders] = useState<FolderTreeNode[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Load initial root folders
  useEffect(() => {
    loadRootFolders();
  }, []);

  const loadRootFolders = async () => {
    try {
      setLoading(true);
      setError(null);
      const folders = await listFolders(undefined);
      setRootFolders(
        folders.map((folder) => ({
          ...folder,
          level: 0,
          expanded: false,
          children: undefined,
        }))
      );
    } catch (err) {
      setError("Failed to load folders");
      console.error("Error loading root folders:", err);
    } finally {
      setLoading(false);
    }
  };

  const loadChildFolders = async (
    parentId: string
  ): Promise<FolderTreeNode[]> => {
    try {
      const folders = await listFolders(parentId);
      return folders.map((folder) => ({
        ...folder,
        level: 0, // Will be set properly when added to parent
        expanded: false,
        children: undefined,
      }));
    } catch (err) {
      console.error("Error loading child folders:", err);
      throw err;
    }
  };

  const updateFolderInTree = useCallback(
    (
      folders: FolderTreeNode[],
      targetId: string,
      updateFn: (folder: FolderTreeNode) => FolderTreeNode
    ): FolderTreeNode[] => {
      return folders.map((folder) => {
        if (folder.id === targetId) {
          return updateFn(folder);
        }
        if (folder.children) {
          return {
            ...folder,
            children: updateFolderInTree(folder.children, targetId, updateFn),
          };
        }
        return folder;
      });
    },
    []
  );

  const toggleFolder = async (folderId: string, currentExpanded: boolean) => {
    if (currentExpanded) {
      // Collapse folder
      setRootFolders((prev) =>
        updateFolderInTree(prev, folderId, (folder) => ({
          ...folder,
          expanded: false,
        }))
      );
    } else {
      // Expand folder - set loading state first
      setRootFolders((prev) =>
        updateFolderInTree(prev, folderId, (folder) => ({
          ...folder,
          loading: true,
        }))
      );

      try {
        const childFolders = await loadChildFolders(folderId);

        setRootFolders((prev) =>
          updateFolderInTree(prev, folderId, (folder) => ({
            ...folder,
            expanded: true,
            loading: false,
            children: childFolders.map((child) => ({
              ...child,
              level: folder.level + 1,
            })),
          }))
        );
      } catch (err) {
        // Handle error - remove loading state
        setRootFolders((prev) =>
          updateFolderInTree(prev, folderId, (folder) => ({
            ...folder,
            loading: false,
          }))
        );
      }
    }
  };

  // Filter folders based on search term
  const filteredFolders = useMemo(() => {
    if (!searchTerm.trim()) return rootFolders;

    const filterFolder = (folder: FolderTreeNode): FolderTreeNode | null => {
      const matchesSearch = folder.name
        .toLowerCase()
        .includes(searchTerm.toLowerCase());
      const filteredChildren =
        (folder.children
          ?.map(filterFolder)
          .filter(Boolean) as FolderTreeNode[]) || [];

      if (matchesSearch || filteredChildren.length > 0) {
        return {
          ...folder,
          children:
            filteredChildren.length > 0 ? filteredChildren : folder.children,
          expanded: filteredChildren.length > 0 ? true : folder.expanded,
        };
      }
      return null;
    };

    return rootFolders.map(filterFolder).filter(Boolean) as FolderTreeNode[];
  }, [rootFolders, searchTerm]);

  const renderFolderNode = (folder: FolderTreeNode) => {
    const isSelected = selectedFolderId === folder.id;
    const isDisabled = disabledFolderId === folder.id;
    const hasChildren = folder.children !== undefined || !folder.expanded;
    const paddingLeft = `${folder.level * 24 + 12}px`;

    return (
      <div key={folder.id} className="relative">
        <div
          className={cn(
            "group relative flex items-center gap-3 py-3 px-4 rounded-lg cursor-pointer",
            "transition-all duration-200 ease-in-out",
            "hover:bg-gradient-to-r hover:from-accent/60 hover:to-accent/40",
            "hover:shadow-md hover:scale-[1.02]",
            "focus-within:ring-2 focus-within:ring-primary/20 focus-within:ring-offset-2",
            isSelected && [
              "bg-gradient-to-r from-primary/15 to-primary/10",
              "border border-primary/30 shadow-lg scale-[1.02]",
              "ring-2 ring-primary/20",
            ],
            isDisabled && [
              "opacity-60 cursor-not-allowed grayscale",
              "hover:bg-transparent hover:shadow-none hover:scale-100",
            ]
          )}
          style={{ paddingLeft }}
          role="treeitem"
          aria-selected={isSelected}
          aria-disabled={isDisabled}
          aria-expanded={hasChildren ? folder.expanded : undefined}
          tabIndex={isDisabled ? -1 : 0}
        >
          {/* Connection Lines for Visual Hierarchy */}
          {folder.level > 0 && (
            <div
              className="absolute left-2 top-0 bottom-0 w-px bg-border/40"
              style={{ left: `${folder.level * 24 - 8}px` }}
            />
          )}

          {/* Expand/Collapse Toggle */}
          <button
            className={cn(
              "relative flex-shrink-0 w-7 h-7 flex items-center justify-center",
              "rounded-md border border-transparent transition-all duration-200",
              "hover:bg-accent hover:border-border hover:shadow-sm",
              "focus:outline-none focus:ring-2 focus:ring-primary/50",
              "active:scale-95",
              !hasChildren && "invisible opacity-0"
            )}
            onClick={(e) => {
              e.stopPropagation();
              if (!folder.loading && hasChildren && !isDisabled) {
                toggleFolder(folder.id, folder.expanded || false);
              }
            }}
            disabled={folder.loading || !hasChildren || isDisabled}
            aria-label={`${folder.expanded ? "Collapse" : "Expand"} ${
              folder.name
            }`}
            tabIndex={isDisabled ? -1 : 0}
          >
            {folder.loading ? (
              <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
            ) : folder.expanded ? (
              <ChevronDown className="w-4 h-4 text-muted-foreground transition-transform" />
            ) : (
              <ChevronRight className="w-4 h-4 text-muted-foreground transition-transform" />
            )}
          </button>

          {/* Folder Icon with Animation */}
          <div
            className={cn(
              "flex-shrink-0 relative transition-all duration-300",
              "group-hover:scale-110",
              isSelected && "scale-110"
            )}
          >
            {folder.expanded ? (
              <FolderOpen className="w-5 h-5 text-blue-600 drop-shadow-sm" />
            ) : (
              <Folder className="w-5 h-5 text-blue-500 drop-shadow-sm" />
            )}
            {isSelected && (
              <div className="absolute -inset-1 bg-primary/20 rounded-full blur-sm -z-10" />
            )}
          </div>

          {/* Folder Name and Content */}
          <div className="flex-1 min-w-0 flex items-center justify-between gap-3">
            <div className="min-w-0 flex-1">
              <span
                className={cn(
                  "block text-sm font-medium truncate transition-colors",
                  "group-hover:text-foreground",
                  isSelected ? "text-foreground" : "text-foreground/80",
                  isDisabled && "text-muted-foreground"
                )}
                title={folder.name}
              >
                {folder.name}
              </span>
              <span className="block text-xs text-muted-foreground mt-0.5">
                {hasChildren && !folder.loading && folder.children
                  ? `${folder.children.length} item${
                      folder.children.length !== 1 ? "s" : ""
                    }`
                  : hasChildren
                  ? "Folder"
                  : "Empty folder"}
              </span>
            </div>

            {/* Select Button with Enhanced Styling */}
            <Button
              size="sm"
              variant={isSelected ? "default" : "ghost"}
              className={cn(
                "h-8 px-3 text-xs font-medium transition-all duration-200",
                "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100",
                "hover:shadow-md active:scale-95",
                "focus:ring-2 focus:ring-primary/50 focus:ring-offset-1",
                isSelected && [
                  "opacity-100 bg-primary text-primary-foreground",
                  "shadow-lg hover:shadow-xl",
                ],
                isDisabled && "cursor-not-allowed opacity-50"
              )}
              disabled={isDisabled}
              onClick={(e) => {
                e.stopPropagation();
                if (!isDisabled) {
                  onSelectFolder(folder);
                }
              }}
              aria-label={`Select ${folder.name} as destination`}
              tabIndex={isDisabled ? -1 : 0}
            >
              {isSelected ? (
                <>
                  <div className="w-2 h-2 bg-current rounded-full mr-2 animate-pulse" />
                  Selected
                </>
              ) : (
                "Select"
              )}
            </Button>
          </div>
        </div>

        {/* Render Children with Smooth Animation */}
        {folder.expanded && folder.children && (
          <div
            className={cn(
              "overflow-hidden transition-all duration-300 ease-in-out",
              "animate-in slide-in-from-top-2 fade-in-50"
            )}
          >
            <div className="space-y-1 mt-1">
              {folder.children.map((child) => renderFolderNode(child))}
            </div>
          </div>
        )}
      </div>
    );
  };

  if (loading && rootFolders.length === 0) {
    return (
      <div
        className={cn(
          "flex flex-col items-center justify-center py-12",
          className
        )}
      >
        <Loader2 className="w-8 h-8 animate-spin text-primary mb-3" />
        <p className="text-sm text-muted-foreground">Loading folders...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className={cn("p-6", className)}>
        <div className="flex items-start gap-3 p-4 bg-destructive/5 border border-destructive/20 rounded-lg">
          <AlertCircle className="w-5 h-5 text-destructive flex-shrink-0 mt-0.5" />
          <div className="flex-1">
            <p className="text-sm font-medium text-destructive mb-1">
              Error loading folders
            </p>
            <p className="text-sm text-destructive/80">{error}</p>
          </div>
        </div>
        <Button
          size="sm"
          variant="outline"
          className="mt-4 w-full"
          onClick={loadRootFolders}
        >
          Try Again
        </Button>
      </div>
    );
  }

  return (
    <div
      className={cn("space-y-2", className)}
      role="tree"
      aria-label="Folder selection tree"
    >
      {/* Root/Home Option with Enhanced Styling */}
      <div
        className={cn(
          "group relative flex items-center gap-3 py-3 px-4 rounded-lg cursor-pointer",
          "transition-all duration-200 ease-in-out",
          "hover:bg-gradient-to-r hover:from-accent/60 hover:to-accent/40",
          "hover:shadow-md hover:scale-[1.02]",
          "focus-within:ring-2 focus-within:ring-primary/20 focus-within:ring-offset-2",
          !selectedFolderId && [
            "bg-gradient-to-r from-primary/15 to-primary/10",
            "border border-primary/30 shadow-lg scale-[1.02]",
            "ring-2 ring-primary/20",
          ]
        )}
        role="treeitem"
        aria-selected={!selectedFolderId}
        tabIndex={0}
      >
        <div className="flex-shrink-0 w-7 h-7" /> {/* Spacer for alignment */}
        {/* Home Icon with Animation */}
        <div
          className={cn(
            "flex-shrink-0 relative transition-all duration-300",
            "group-hover:scale-110",
            !selectedFolderId && "scale-110"
          )}
        >
          <Home className="w-5 h-5 text-gray-600 drop-shadow-sm" />
          {!selectedFolderId && (
            <div className="absolute -inset-1 bg-primary/20 rounded-full blur-sm -z-10" />
          )}
        </div>
        <div className="flex-1 min-w-0 flex items-center justify-between gap-3">
          <div className="min-w-0 flex-1">
            <span
              className={cn(
                "block text-sm font-medium transition-colors",
                "group-hover:text-foreground",
                !selectedFolderId ? "text-foreground" : "text-foreground/80"
              )}
            >
              Home (Root)
            </span>
            <span className="block text-xs text-muted-foreground mt-0.5">
              Move to root directory
            </span>
          </div>

          <Button
            size="sm"
            variant={!selectedFolderId ? "default" : "ghost"}
            className={cn(
              "h-8 px-3 text-xs font-medium transition-all duration-200",
              "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100",
              "hover:shadow-md active:scale-95",
              "focus:ring-2 focus:ring-primary/50 focus:ring-offset-1",
              !selectedFolderId && [
                "opacity-100 bg-primary text-primary-foreground",
                "shadow-lg hover:shadow-xl",
              ]
            )}
            onClick={() => onSelectFolder(null)}
            aria-label="Select root directory as destination"
          >
            {!selectedFolderId ? (
              <>
                <div className="w-2 h-2 bg-current rounded-full mr-2 animate-pulse" />
                Selected
              </>
            ) : (
              "Select"
            )}
          </Button>
        </div>
      </div>

      {/* Folder Tree */}
      <div className="space-y-1">
        {filteredFolders.map((folder) => renderFolderNode(folder))}
      </div>

      {/* Empty State */}
      {filteredFolders.length === 0 && !loading && (
        <div className="py-8 text-center">
          <Folder className="w-12 h-12 text-muted-foreground/40 mx-auto mb-3" />
          <p className="text-sm font-medium text-muted-foreground mb-1">
            {searchTerm ? "No matching folders" : "No folders found"}
          </p>
          <p className="text-xs text-muted-foreground">
            {searchTerm
              ? "Try adjusting your search terms"
              : "Create a folder to get started"}
          </p>
        </div>
      )}
    </div>
  );
}
