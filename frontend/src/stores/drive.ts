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
  setItems: (items: DriveItem[]) => void;
  setViewMode: (m: "grid" | "list") => void;
  setSearchQuery: (q: string) => void;
  setCurrentPath: (path: string[]) => void;
  toggleItemSelection: (id: string) => void;
  clearSelection: () => void;
};

export const useDriveStore = create<DriveState>((set, get) => ({
  items: [],
  viewMode: "grid",
  searchQuery: "",
  currentPath: [],
  selectedItems: [],

  setItems: (items) => set({ items }),
  setViewMode: (m) => set({ viewMode: m }),
  setSearchQuery: (q) => set({ searchQuery: q }),
  setCurrentPath: (path) => set({ currentPath: path }),
  toggleItemSelection: (id) => {
    const { selectedItems } = get();
    if (selectedItems.includes(id)) {
      set({ selectedItems: selectedItems.filter((i) => i !== id) });
    } else {
      set({ selectedItems: [...selectedItems, id] });
    }
  },
  clearSelection: () => set({ selectedItems: [] }),
}));
