import React, { useEffect, useState } from "react";
import { useParams, Navigate } from "react-router-dom";
import { motion } from "framer-motion";
import axios from "@/lib/axios";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Download, FileText } from "lucide-react";
import { downloadUrlToFile } from "@/lib/utils";

type FolderFile = {
  id: string;
  filename: string;
  size: number;
  download: string;
};

type FolderShareResp = { type: "folder"; files: FolderFile[] };

export default function ShareFolderView() {
  const { token } = useParams<{ token: string }>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<FolderShareResp | null>(null);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;

    const fetchShare = async () => {
      setLoading(true);
      setError(null);
      try {
        const res = await axios.get(`/api/v1/s/${token}`);
        if (!cancelled) {
          if (res.data && res.data.type === "folder")
            setData(res.data as FolderShareResp);
          else setError("Shared token did not return a folder");
        }
      } catch (err: unknown) {
        if (!cancelled) {
          const respStatus = (err as { response?: { status?: number } })
            ?.response?.status;
          const message = (err as { message?: string })?.message;
          if (respStatus === 404) setError("Share not found");
          else if (respStatus === 410) setError("Share expired");
          else setError(message || "Failed to load share");
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    fetchShare();
    return () => {
      cancelled = true;
    };
  }, [token]);

  if (!token) return <Navigate to="/dashboard/home" replace />;

  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25 }}
      className="max-w-4xl mx-auto p-6"
    >
      {loading ? (
        <div className="text-center py-12">Loading...</div>
      ) : error ? (
        <div className="text-center py-12">
          <h2 className="text-xl font-semibold mb-2">{error}</h2>
          <p className="text-muted-foreground">Unable to open shared folder.</p>
        </div>
      ) : data ? (
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Shared Folder</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground mb-4">
                Files are shown as they existed at the time of sharing. Nested
                subfolders are not included unless the backend included them in
                the share response.
              </p>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                {data.files && data.files.length > 0 ? (
                  data.files.map((f) => (
                    <Card key={f.id}>
                      <CardContent className="p-4">
                        <div className="flex items-center justify-between">
                          <div>
                            <div className="font-medium truncate">
                              {f.filename}
                            </div>
                            <div className="text-xs text-muted-foreground">
                              {Math.round(f.size / 1024)} KB
                            </div>
                          </div>
                          <div>
                            <Button
                              onClick={async () => {
                                try {
                                  await downloadUrlToFile(
                                    f.download,
                                    f.filename
                                  );
                                } catch (err) {
                                  // fallback to opening URL
                                  console.error(
                                    "Download failed, opening URL",
                                    err
                                  );
                                  window.open(
                                    f.download,
                                    "_blank",
                                    "noopener,noreferrer"
                                  );
                                }
                              }}
                            >
                              <Download className="w-4 h-4 mr-2" />
                              Download
                            </Button>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  ))
                ) : (
                  <div className="text-muted-foreground py-6 text-center">
                    No files in this shared folder
                  </div>
                )}
              </div>
            </CardContent>
          </Card>
        </div>
      ) : null}
    </motion.div>
  );
}
