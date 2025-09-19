import axios from "../lib/axios";

export type FileItem = {
  id: string;
  name: string;
  filename: string;
  type: "file";
  mimeType?: string;
  mime?: string;
  size?: number;
  createdAt: string;
  updatedAt: string;
  isShared: boolean;
  isStarred: boolean;
  isTrashed: boolean;
  downloadCount: number;
  tags: string[];
  version: number;
  contentHash?: string;
  physicalSize?: number;
  refCount?: number;
  dedupSavings?: number;
  folderId?: string;
};

export async function listFiles(
  folderId?: string,
  page = 1,
  limit = 20
): Promise<FileItem[]> {
  const res = await axios.get("/api/v1/files", {
    params: { folderId, page, limit },
  });
  // Backend returns { files: [...] }, extract and transform the files array
  const backendFiles = res.data.files || [];
  return backendFiles.map(
    (file: {
      id: string;
      filename: string;
      mime: string;
      size: number;
      createdAt: string;
      updatedAt: string;
      downloadCount: number;
      contentHash?: string;
      physicalSize?: number;
      refCount?: number;
      dedupSavings?: number;
      folderId?: string;
    }) => ({
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
      folderId: file.folderId,
    })
  );
}

export async function getFile(id: string): Promise<FileItem> {
  const res = await axios.get(`/api/v1/files/${id}`);
  return res.data;
}

export async function deleteFile(id: string): Promise<void> {
  await axios.delete(`/api/v1/files/${id}`);
}

export async function patchFile(
  id: string,
  body: { filename?: string; tags?: string[] }
): Promise<void> {
  await axios.patch(`/api/v1/files/${id}`, body);
}
