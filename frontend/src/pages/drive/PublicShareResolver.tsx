import React, { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { motion } from "framer-motion";
import PublicShareView from "./PublicShareView"; // folder view
import PublicFileView from "./PublicFileView"; // file view
import {
  publicShareApi,
  type ResolvedPublicFile,
  type ResolvedPublicFolder,
} from "@/api/operations";

export default function PublicShareResolver() {
  const { token } = useParams<{ token: string }>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [type, setType] = useState<"file" | "folder" | null>(null);
  const [file, setFile] = useState<ResolvedPublicFile | null>(null);
  const [folderTokenForView, setFolderTokenForView] = useState<string | null>(
    null
  );

  const isFileRes = (
    v: unknown
  ): v is ResolvedPublicFile & { type: "file" } => {
    return (
      typeof v === "object" &&
      v !== null &&
      (v as Record<string, unknown>)["type"] === "file"
    );
  };

  const isFolderRes = (
    v: unknown
  ): v is ResolvedPublicFolder & { type: "folder" } => {
    return (
      typeof v === "object" &&
      v !== null &&
      (v as Record<string, unknown>)["type"] === "folder"
    );
  };

  useEffect(() => {
    if (!token) return;
    setLoading(true);
    setError(null);
    (async () => {
      try {
        const res = await publicShareApi.resolveShare(token);
        if (isFileRes(res)) {
          setType("file");
          setFile(res);
        } else if (isFolderRes(res)) {
          setType("folder");
          setFolderTokenForView(token);
        } else {
          setError("Unknown share type");
        }
      } catch (err) {
        setError((err as Error).message);
      } finally {
        setLoading(false);
      }
    })();
  }, [token]);

  if (!token) return null;

  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25 }}
    >
      {loading ? (
        <div className="text-center py-12">Loading share...</div>
      ) : error ? (
        <div className="text-center py-12">{error}</div>
      ) : type === "file" && file ? (
        <PublicFileView file={file} />
      ) : type === "folder" && folderTokenForView ? (
        <PublicShareView />
      ) : (
        <div className="text-center py-12">Invalid share</div>
      )}
    </motion.div>
  );
}
