import React, { useEffect, useState } from "react";
import { useParams, Navigate } from "react-router-dom";
import { motion } from "framer-motion";
import axios from "@/lib/axios";
import { Button } from "@/components/ui/button";
import { downloadUrlToFile } from "@/lib/utils";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Download,
  FileText,
  Music,
  Video,
  Image,
  FileArchive,
} from "lucide-react";

type FileShareResp = {
  type: "file";
  fileId: string;
  filename: string;
  size: number;
  download: string;
};

type FolderFile = {
  id: string;
  filename: string;
  size: number;
  download: string;
};

type FolderShareResp = { type: "folder"; files: FolderFile[] };

type ShareResp = FileShareResp | FolderShareResp;

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

const FilePreview: React.FC<{
  filename: string;
  downloadUrl: string;
  size: number;
}> = ({ filename, downloadUrl, size }) => {
  const fileType = getFileType(filename);
  const [previewError, setPreviewError] = useState(false);
  const [isDownloading, setIsDownloading] = useState(false);

  const renderPreview = () => {
    if (previewError) {
      return (
        <div className="bg-muted rounded-lg p-8 text-center">
          <FileText className="w-12 h-12 mx-auto mb-2 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">Preview not available</p>
        </div>
      );
    }

    switch (fileType) {
      case "image":
        return (
          <div className="bg-muted rounded-lg p-4">
            <img
              src={downloadUrl}
              alt={filename}
              className="max-w-full max-h-96 mx-auto rounded"
              onError={() => setPreviewError(true)}
            />
          </div>
        );

      case "video":
        return (
          <div className="bg-muted rounded-lg p-4">
            <video
              controls
              className="max-w-full max-h-96 mx-auto rounded"
              onError={() => setPreviewError(true)}
            >
              <source src={downloadUrl} />
              Your browser does not support video playback.
            </video>
          </div>
        );

      case "audio":
        return (
          <div className="bg-muted rounded-lg p-8 text-center">
            <Music className="w-12 h-12 mx-auto mb-4 text-muted-foreground" />
            <audio
              controls
              className="w-full max-w-md mx-auto"
              onError={() => setPreviewError(true)}
            >
              <source src={downloadUrl} />
              Your browser does not support audio playback.
            </audio>
          </div>
        );

      case "pdf":
        return (
          <div className="bg-muted rounded-lg p-4">
            <iframe
              src={downloadUrl}
              className="w-full h-96 rounded"
              title={filename}
              onError={() => setPreviewError(true)}
            />
          </div>
        );

      case "text":
        return (
          <div className="bg-muted rounded-lg p-8 text-center">
            <FileText className="w-12 h-12 mx-auto mb-2 text-muted-foreground" />
            <p className="text-sm text-muted-foreground mb-4">
              Text file preview
            </p>
            <p className="text-xs text-muted-foreground">
              Download to view contents
            </p>
          </div>
        );

      case "office":
        return (
          <div className="bg-muted rounded-lg p-8 text-center">
            <FileText className="w-12 h-12 mx-auto mb-2 text-muted-foreground" />
            <p className="text-sm text-muted-foreground mb-4">
              Office document
            </p>
            <p className="text-xs text-muted-foreground">
              Download to view in your preferred application
            </p>
          </div>
        );

      case "archive":
        return (
          <div className="bg-muted rounded-lg p-8 text-center">
            <FileArchive className="w-12 h-12 mx-auto mb-2 text-muted-foreground" />
            <p className="text-sm text-muted-foreground mb-4">Archive file</p>
            <p className="text-xs text-muted-foreground">
              Download to extract contents
            </p>
          </div>
        );

      default:
        return (
          <div className="bg-muted rounded-lg p-8 text-center">
            <FileText className="w-12 h-12 mx-auto mb-2 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">
              No preview available
            </p>
          </div>
        );
    }
  };

  return (
    <div className="space-y-4">
      {renderPreview()}
      <div className="flex items-center justify-between p-4 bg-background border rounded-lg">
        <div>
          <h3 className="font-medium">{filename}</h3>
          <p className="text-sm text-muted-foreground">
            {Math.round(size / 1024)} KB • {fileType}
          </p>
        </div>
        <div className="inline-block">
          <Button
            onClick={async () => {
              try {
                setIsDownloading(true);
                await downloadUrlToFile(downloadUrl, filename);
              } catch (err) {
                // If fetch/save fails (CORS, network), fallback to opening the URL
                // in a new tab so user can still download via the presigned link.
                // eslint-disable-next-line no-console
                console.error(
                  "Programmatic download failed, falling back:",
                  err
                );
                window.open(downloadUrl, "_blank", "noopener,noreferrer");
              } finally {
                setIsDownloading(false);
              }
            }}
            disabled={isDownloading}
          >
            <Download className="w-4 h-4 mr-2" />
            {isDownloading ? "Downloading..." : "Download"}
          </Button>
        </div>
      </div>
    </div>
  );
};

export default function PublicShareView() {
  const { token } = useParams<{ token: string }>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<ShareResp | null>(null);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;

    const fetchShare = async () => {
      setLoading(true);
      setError(null);
      try {
        const res = await axios.get(`/api/v1/s/${token}`);
        if (!cancelled) {
          setData(res.data as ShareResp);
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

  if (!token) return <Navigate to="/" replace />;

  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25 }}
      className="max-w-3xl mx-auto p-6"
    >
      {loading ? (
        <div className="text-center py-12">Loading...</div>
      ) : error ? (
        <div className="text-center py-12">
          <h2 className="text-xl font-semibold mb-2">{error}</h2>
          <p className="text-muted-foreground">Unable to open shared item.</p>
        </div>
      ) : data ? (
        <div className="space-y-6">
          {data.type === "file" ? (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <FileText className="w-5 h-5" />
                  Shared File
                </CardTitle>
              </CardHeader>
              <CardContent>
                <FilePreview
                  filename={data.filename}
                  downloadUrl={data.download}
                  size={data.size}
                />
              </CardContent>
            </Card>
          ) : data.type === "folder" ? (
            <Card>
              <CardHeader>
                <CardTitle>Shared Folder</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-2">
                  {data.files && data.files.length > 0 ? (
                    data.files.map((f) => (
                      <div key={f.id} className="space-y-4">
                        <FilePreview
                          filename={f.filename}
                          downloadUrl={f.download}
                          size={f.size}
                        />
                      </div>
                    ))
                  ) : (
                    <div className="text-muted-foreground py-6 text-center">
                      No files in this folder
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          ) : (
            <div className="text-center">Unsupported share type</div>
          )}
        </div>
      ) : null}
    </motion.div>
  );
}
