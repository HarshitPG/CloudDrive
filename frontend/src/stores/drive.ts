import { create } from "zustand";
import { type FileItem } from "../api/files";
import { type FolderItem } from "../api/folders";

export type DriveItem = FileItem | FolderItem;

type DriveState = {
  items: DriveItem[];
  viewMode: "grid" | "list";
  searchQuery: string;
  currentPath: string[];
  selectedItems: string[];
  currentFolderId?: string;
  isLoading: boolean;
  setItems: (items: DriveItem[]) => void;
  addItem: (item: DriveItem) => void;
  updateItem: (id: string, updates: Partial<DriveItem>) => void;
  removeItem: (id: string) => void;
  setViewMode: (m: "grid" | "list") => void;
  setSearchQuery: (q: string) => void;
  setCurrentPath: (path: string[]) => void;
  setCurrentFolderId: (id?: string) => void;
  setIsLoading: (loading: boolean) => void;
  toggleItemSelection: (id: string) => void;
  clearSelection: () => void;
  getFolders: () => FolderItem[];
  getFiles: () => FileItem[];
};

export const useDriveStore = create<DriveState>((set, get) => ({
  items: [],
  viewMode: "grid",
  searchQuery: "",
  currentPath: [],
  selectedItems: [],
  currentFolderId: undefined,
  isLoading: false,

  setItems: (items) => set({ items }),
  addItem: (item) => set((state) => ({ items: [...state.items, item] })),
  updateItem: (id, updates) =>
    set((state) => ({
      items: state.items.map((item) => {
        if (item.id !== id) return item;

        if (item.type === "folder" && updates.type !== "file") {
          return { ...item, ...updates } as FolderItem;
        } else if (item.type === "file" && updates.type !== "folder") {
          return { ...item, ...updates } as FileItem;
        }

        return item;
      }),
    })),
  removeItem: (id) =>
    set((state) => ({
      items: state.items.filter((item) => item.id !== id),
    })),
  setViewMode: (m) => set({ viewMode: m }),
  setSearchQuery: (q) => set({ searchQuery: q }),
  setCurrentPath: (path) => set({ currentPath: path }),
  setCurrentFolderId: (id) => set({ currentFolderId: id }),
  setIsLoading: (loading) => set({ isLoading: loading }),
  toggleItemSelection: (id) => {
    const { selectedItems } = get();
    if (selectedItems.includes(id)) {
      set({ selectedItems: selectedItems.filter((i) => i !== id) });
    } else {
      set({ selectedItems: [...selectedItems, id] });
    }
  },
  clearSelection: () => set({ selectedItems: [] }),
  getFolders: () =>
    get().items.filter((item) => item.type === "folder") as FolderItem[],
  getFiles: () =>
    get().items.filter((item) => item.type === "file") as FileItem[],
}));
