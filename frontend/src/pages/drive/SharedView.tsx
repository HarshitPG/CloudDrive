import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { motion } from "framer-motion";
import {
  Users,
  Folder,
  FileText,
  Download,
  Eye,
  ChevronRight,
  Home as HomeIcon,
} from "lucide-react";
import Breadcrumbs, { type Crumb } from "@/components/ui/Breadcrumbs";
import axios from "@/lib/axios";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { sharedApi } from "@/api/operations";
import { downloadUrlToFile } from "@/lib/utils";
import FullscreenPreviewModal from "@/components/FullscreenPreviewModal";

type SharedFile = {
  id: string;
  filename: string;
  mime: string;
  size: number;
  createdAt: string;
  updatedAt: string;
  downloadCount: number;
};

type SharedFolder = {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
};

export default function SharedView() {
  const navigate = useNavigate();
  const { folderId } = useParams<{ folderId: string }>();
  const [files, setFiles] = useState<SharedFile[]>([]);
  const [folders, setFolders] = useState<SharedFolder[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [breadcrumbs, setBreadcrumbs] = useState<Crumb[]>([]);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewUrl, setPreviewUrl] = useState<string | undefined>(undefined);
  const [previewFilename, setPreviewFilename] = useState<string | undefined>(
    undefined
  );
  const [previewMimeType, setPreviewMimeType] = useState<string | undefined>(
    undefined
  );

  useEffect(() => {
    let abort = false;
    async function loadRoot() {
      try {
        setLoading(true);
        setError(null);
        if (folderId) {
          // Fetch nested contents of a shared folder the user has access to
          const res = await axios.get(
            `/api/v1/shares/folders/${folderId}/contents`
          );
          if (!abort) {
            setFiles(res.data.files || []);
            setFolders(res.data.folders || []);
            const cur = res.data.current as
              | { id: string; name: string; parentId?: string | null }
              | undefined;
            if (cur) {
              // Request the canonical ancestor chain for this shared folder
              try {
                const ancRes = await axios.get(
                  `/api/v1/shares/folders/${cur.id}/ancestors`
                );
                const anc = ancRes.data.ancestors || [];
                setBreadcrumbs([{ name: "Shared" }, ...anc]);
              } catch (e) {
                // Fallback to basic crumbs
                const crumbs: { id?: string; name: string }[] = [
                  { name: "Shared" },
                ];
                if (cur.parentId)
                  crumbs.push({ id: cur.parentId, name: "..." });
                crumbs.push({ id: cur.id, name: cur.name || "Current" });
                setBreadcrumbs(crumbs);
              }
            } else {
              setBreadcrumbs([
                { name: "Shared" },
                { id: folderId, name: "Current" },
              ]);
            }
          }
        } else {
          const res = await sharedApi.listSharedWithMe(50, 0);
          if (!abort) {
            setFiles(res.files || []);
            setFolders(res.folders || []);
            setBreadcrumbs([{ name: "Shared" }]);
          }
        }
      } catch (e) {
        if (!abort)
          setError(
            e instanceof Error ? e.message : "Failed to load shared items"
          );
      } finally {
        if (!abort) setLoading(false);
      }
    }
    loadRoot();
    return () => {
      abort = true;
    };
  }, [folderId]);

  const handleOpenFolder = (f: SharedFolder) => {
    navigate(`/dashboard/shared/folder/${f.id}`);
  };

  const handleBreadcrumbClick = (id?: string) => {
    if (!id) navigate(`/dashboard/shared`);
    else navigate(`/dashboard/shared/folder/${id}`);
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="space-y-6"
    >
      <div>
        <h1 className="text-2xl font-bold text-foreground mb-2">
          Shared with me
        </h1>
        <p className="text-muted-foreground">
          Files and folders shared by others
        </p>
        {breadcrumbs.length > 0 && (
          <div className="flex items-center gap-2 mt-2">
            <Breadcrumbs
              crumbs={breadcrumbs as Crumb[]}
              onClick={handleBreadcrumbClick}
            />
          </div>
        )}
      </div>

      {loading ? (
        <div className="text-center py-12">Loading...</div>
      ) : error ? (
        <div className="text-center py-12">
          <h3 className="text-lg font-medium text-foreground mb-2">{error}</h3>
          <p className="text-muted-foreground">
            We couldn’t load shared items. Please try again.
          </p>
        </div>
      ) : files.length === 0 && folders.length === 0 ? (
        <div className="text-center py-12">
          <div className="w-16 h-16 bg-muted rounded-full flex items-center justify-center mx-auto mb-4">
            <Users className="w-8 h-8 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium text-foreground mb-2">
            No shared items
          </h3>
          <p className="text-muted-foreground">
            Files or folders shared with you will appear here
          </p>
        </div>
      ) : (
        <>
          <div className="space-y-8">
            {folders.length > 0 && (
              <section>
                <h2 className="text-lg font-semibold mb-3">Folders</h2>
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                  {folders.map((f) => (
                    <Card
                      key={f.id}
                      className="drive-card hover:shadow-sm transition"
                      onClick={() => handleOpenFolder(f)}
                    >
                      <CardContent className="p-4">
                        <div className="flex items-start justify-between mb-3">
                          <div className="w-10 h-10 rounded-lg flex items-center justify-center bg-primary/10 text-primary">
                            <Folder className="w-5 h-5" />
                          </div>
                          <div className="flex items-center gap-1">
                            <Button
                              variant="ghost"
                              className="h-8 w-8 p-0"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleOpenFolder(f);
                              }}
                            >
                              <Eye className="w-4 h-4" />
                            </Button>
                          </div>
                        </div>

                        <h3 className="font-medium text-foreground text-sm mb-2 truncate">
                          {f.name}
                        </h3>

                        <div className="space-y-2">
                          <div className="flex items-center justify-between text-xs text-muted-foreground">
                            <span>
                              {new Date(f.createdAt).toLocaleDateString()}
                            </span>
                            <span>-</span>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              </section>
            )}

            {files.length > 0 && (
              <section>
                <h2 className="text-lg font-semibold mb-3">Files</h2>
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                  {files.map((file) => (
                    <Card
                      key={file.id}
                      className="drive-card hover:shadow-sm transition"
                    >
                      <CardContent className="p-4">
                        <div className="flex items-start justify-between mb-3">
                          <div
                            className={`w-10 h-10 rounded-lg flex items-center justify-center bg-muted text-muted-foreground`}
                          >
                            <FileText className="w-5 h-5" />
                          </div>
                          <div className="flex items-center gap-1">
                            <Button
                              variant="ghost"
                              className="h-8 w-8 p-0"
                              onClick={async (e) => {
                                e.stopPropagation();
                                try {
                                  const res = await axios.get(
                                    `/api/v1/shares/files/${file.id}/download`
                                  );
                                  const url = res.data.downloadUrl as string;
                                  await downloadUrlToFile(url, file.filename);
                                } catch (err) {
                                  console.error(err);
                                }
                              }}
                            >
                              <Download className="w-4 h-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              className="h-8 w-8 p-0"
                              onClick={async (e) => {
                                e.stopPropagation();
                                try {
                                  const res = await axios.get(
                                    `/api/v1/shares/files/${file.id}/download`
                                  );
                                  const url = res.data.downloadUrl as string;
                                  setPreviewUrl(url);
                                  setPreviewFilename(file.filename);
                                  setPreviewMimeType(file.mime);
                                  setPreviewOpen(true);
                                } catch (err) {
                                  console.error(err);
                                }
                              }}
                            >
                              <Eye className="w-4 h-4" />
                            </Button>
                          </div>
                        </div>

                        <h3 className="font-medium text-foreground text-sm mb-2 truncate">
                          {file.filename}
                        </h3>

                        <div className="space-y-2">
                          <div className="flex items-center justify-between text-xs text-muted-foreground">
                            <span>
                              {new Date(file.createdAt).toLocaleDateString()}
                            </span>
                            <span>
                              {Math.round((file.size || 0) / 1024)} KB
                            </span>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              </section>
            )}
          </div>
          <FullscreenPreviewModal
            open={previewOpen}
            onClose={() => {
              setPreviewOpen(false);
              setPreviewUrl(undefined);
              setPreviewFilename(undefined);
              setPreviewMimeType(undefined);
            }}
            url={previewUrl}
            filename={previewFilename}
            mimeType={previewMimeType}
          />
        </>
      )}
    </motion.div>
  );
}
