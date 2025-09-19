import { useEffect } from "react";
import { Search } from "lucide-react";
import { Input } from "../../components/ui/input";
import DashboardLayout from "../../components/DashboardLayout";
import { useDriveStore, type DriveItem } from "../../stores/drive";
import { listFiles } from "../../api/files";
import { listFolders } from "../../api/folders";
import { searchFiles } from "../../api/search";

export default function Home() {
  const { items, setItems, searchQuery, setSearchQuery } = useDriveStore();

  // Load root on first render
  useEffect(() => {
    let isMounted = true;

    async function loadRoot() {
      try {
        // Load files first, then folders with a small delay to avoid rate limiting
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

    // Add a small delay before starting to avoid conflicts with other components
    const timer = setTimeout(loadRoot, 100);

    return () => {
      isMounted = false;
      clearTimeout(timer);
    };
  }, [setItems]);

  useEffect(() => {
    if (!searchQuery.trim()) return;

    let isMounted = true;

    // Debounce search requests to avoid rate limiting
    const timer = setTimeout(async () => {
      try {
        const res = await searchFiles(searchQuery);
        if (!isMounted) return;

        const driveItems: DriveItem[] = res.map((item) => {
          if (item.type === "folder") {
            return {
              id: item.id,
              name: item.name,
              type: "folder" as const,
              createdAt: "",
              updatedAt: "",
            };
          } else {
            return {
              id: item.id,
              name: item.name,
              filename: item.name,
              type: "file" as const,
              mimeType: "",
              mime: "",
              size: 0,
              createdAt: "",
              updatedAt: "",
              isShared: false,
              isStarred: false,
              isTrashed: false,
              downloadCount: 0,
              tags: [],
              version: 1,
            };
          }
        });
        setItems(driveItems);
      } catch (error) {
        console.error("Search failed:", error);
        if (isMounted) setItems([]);
      }
    }, 300);

    return () => {
      isMounted = false;
      clearTimeout(timer);
    };
  }, [searchQuery, setItems]);

  return (
    <DashboardLayout>
      {/* Search bar */}
      <div className="flex items-center gap-2 mb-4">
        <Search className="w-5 h-5 text-gray-400" />
        <Input
          placeholder="Search files and folders..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />
      </div>

      {/* File/folder list */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {items.map((item) => (
          <div key={item.id} className="border p-3 rounded">
            <p className="font-medium">{item.name}</p>
            <p className="text-xs text-gray-500">{item.type}</p>
          </div>
        ))}
      </div>
    </DashboardLayout>
  );
}
