import { useState, useEffect } from "react";
import { motion } from "framer-motion";
import {
  Menu,
  X,
  Users,
  Clock,
  Star,
  Trash2,
  Folder,
  Home,
  Upload,
  ChevronDown,
} from "lucide-react";
import { useNavigate, useLocation } from "react-router-dom";
import QuotaWidget from "../QuotaWidget";
import { uploadManager } from "@/lib/uploadManager";
import { useRef } from "react";
import { pickDirectoryFiles, enqueueFolderUploads } from "@/lib/folderUpload";

export default function SidebarToggle() {
  const [isOpen, setIsOpen] = useState(true);
  const [isMobile, setIsMobile] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const detailsRef = useRef<HTMLDetailsElement | null>(null);
  const [uploadOpen, setUploadOpen] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    const handleResize = () => {
      const mobile = window.innerWidth < 768;
      setIsMobile(mobile);
      setIsOpen(!mobile);
    };

    handleResize();
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  const toggleSidebar = () => setIsOpen(!isOpen);

  return (
    <motion.aside
      animate={{ width: isOpen ? 240 : 64 }}
      transition={{ duration: 0.3, ease: "easeInOut" }}
      className="h-screen bg-sidebar shadow-md flex flex-col border-r border-sidebar-border overflow-hidden"
    >
      <div
        className={`flex items-center p-4 text-lg font-semibold ${
          isOpen ? "justify-between" : "justify-center"
        }`}
      >
        {isOpen && (
          <div className="flex items-center gap-2">
            <Folder className="text-drive-blue shrink-0" size={22} />
            <span className="truncate hidden md:inline">CloudDrive</span>
          </div>
        )}

        <button
          onClick={toggleSidebar}
          className="p-1 rounded-md hover:bg-sidebar-accent transition shrink-0"
        >
          {isOpen ? <X size={20} /> : <Menu size={22} />}
        </button>
      </div>

      <nav className="flex-1 px-2">
        {/* Upload action */}
        <div className="mb-2">
          <input
            ref={fileInputRef}
            type="file"
            className="hidden"
            multiple
            onChange={(e) => {
              if (e.target.files && e.target.files.length) {
                uploadManager.addFiles(Array.from(e.target.files));
                e.currentTarget.value = "";
              }
            }}
          />
          <div className="relative">
            <details
              ref={detailsRef}
              onToggle={() => setUploadOpen(Boolean(detailsRef.current?.open))}
              className="group"
            >
              <summary
                className={`flex items-center gap-3 px-3 py-2 rounded-md cursor-pointer transition hover:bg-sidebar-accent text-sidebar-foreground list-none`}
              >
                <div className="shrink-0">
                  <Upload size={20} />
                </div>
                {isOpen && !isMobile && (
                  <span className="truncate flex items-center gap-2">
                    <span>Upload</span>
                    <ChevronDown
                      size={22}
                      className={`transition-transform duration-150 ease-in-out text-sidebar-foreground ${
                        uploadOpen ? "rotate-180" : ""
                      }`}
                    />
                  </span>
                )}
              </summary>
              <div className="absolute z-10 mt-1 w-44 rounded-md border border-sidebar-border bg-white shadow-lg">
                <button
                  className="block w-full text-left px-3 py-2 text-sm hover:bg-sidebar-accent"
                  onClick={() => {
                    fileInputRef.current?.click();
                  }}
                >
                  Upload files
                </button>
                <button
                  className="block w-full text-left px-3 py-2 text-sm hover:bg-sidebar-accent"
                  onClick={async () => {
                    try {
                      const { entries, rootName } = await pickDirectoryFiles();
                      if (!entries.length) return;
                      const res = await enqueueFolderUploads(rootName, entries);
                      navigate(`/dashboard/home/folder/${res.rootFolderId}`);
                    } catch (e) {
                      //
                    }
                  }}
                >
                  Upload folder
                </button>
              </div>
            </details>
          </div>
        </div>

        <SidebarItem
          icon={<Home size={20} />}
          label="My Drive"
          isOpen={isOpen && !isMobile}
          path="/dashboard/home"
        />
        <SidebarItem
          icon={<Users size={20} />}
          label="Shared with me"
          isOpen={isOpen && !isMobile}
          path="/dashboard/shared"
        />
        <SidebarItem
          icon={<Clock size={20} />}
          label="Recent"
          isOpen={isOpen && !isMobile}
          path="/dashboard/recent"
        />
        <SidebarItem
          icon={<Star size={20} />}
          label="Starred"
          isOpen={isOpen && !isMobile}
          path="/dashboard/starred"
        />
        <SidebarItem
          icon={<Trash2 size={20} />}
          label="Trash"
          isOpen={isOpen && !isMobile}
          path="/dashboard/trash"
        />
      </nav>

      {/* Footer */}
      <div className="p-4 text-sm border-t border-sidebar-border">
        {isOpen && !isMobile && (
          <>
            <QuotaWidget />
          </>
        )}
      </div>
    </motion.aside>
  );
}

function SidebarItem({
  icon,
  label,
  isOpen,
  path,
  active = false,
}: {
  icon: React.ReactNode;
  label: string;
  isOpen: boolean;
  path: string;
  active?: boolean;
}) {
  const navigate = useNavigate();
  const location = useLocation();

  const isActive =
    location.pathname === path || location.pathname.startsWith(path);

  const handleClick = () => {
    navigate(path);
  };

  return (
    <div
      onClick={handleClick}
      className={`flex items-center gap-3 px-3 py-2 rounded-md cursor-pointer transition ${
        isActive
          ? "bg-drive-blue text-black font-semibold"
          : "text-sidebar-foreground hover:bg-sidebar-accent"
      }`}
    >
      <div className="shrink-0">{icon}</div>
      {isOpen && (
        <span className="truncate" aria-current={isActive ? "page" : undefined}>
          {label}
        </span>
      )}
    </div>
  );
}
