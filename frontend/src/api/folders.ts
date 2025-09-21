import axios from "../lib/axios";

export type FolderItem = {
  id: string;
  name: string;
  type: "folder";
  createdAt: string;
  updatedAt: string;
};

export type FolderListResponse = {
  folders: FolderItem[];
  page: {
    limit: number;
    offset: number;
  };
};

export type FolderTree = {
  id: string;
  name: string;
  createdAt: string;
  children: FolderTree[];
};

export async function listFolders(
  parentId?: string,
  limit: number = 20,
  offset: number = 0
): Promise<FolderItem[]> {
  const params = new URLSearchParams({
    limit: limit.toString(),
    offset: offset.toString(),
  });

  if (parentId) {
    params.set("parentId", parentId);
  }

  const res = await axios.get<FolderListResponse>(`/api/v1/folders?${params}`);
  return (res.data.folders || []).map((folder) => ({
    ...folder,
    type: "folder" as const,
  }));
}

export async function createFolder(
  name: string,
  parentId?: string
): Promise<{ message: string }> {
  const res = await axios.post("/api/v1/folders", {
    name,
    parentId: parentId || undefined,
  });
  return res.data;
}

export async function renameFolder(
  id: string,
  name: string
): Promise<{ message: string }> {
  const res = await axios.patch(`/api/v1/folders/${id}`, {
    name,
  });
  return res.data;
}

export async function deleteFolder(
  id: string,
  permanent: boolean = false
): Promise<{ message: string }> {
  const params = permanent ? "?permanent=true" : "";
  const res = await axios.delete(`/api/v1/folders/${id}${params}`);
  return res.data;
}

export async function getFolderContents(id: string): Promise<{
  folders: FolderItem[];
  files: {
    id: string;
    filename: string;
    mime: string;
    size: number;
    createdAt: string;
  }[];
}> {
  const res = await axios.get(`/api/v1/folders/${id}/contents`);
  return {
    folders: (res.data.folders || []).map(
      (folder: { id: string; name: string; created_at: string }) => ({
        id: folder.id,
        name: folder.name,
        type: "folder" as const,
        createdAt: folder.created_at,
        updatedAt: folder.created_at,
      })
    ),
    files: res.data.files || [],
  };
}

export async function getFolderFiles(
  folderId: string,
  limit: number = 20,
  offset: number = 0
): Promise<
  {
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
  }[]
> {
  const params = new URLSearchParams({
    limit: limit.toString(),
    offset: offset.toString(),
  });

  const res = await axios.get(`/api/v1/folders/${folderId}/files?${params}`);
  return res.data.files || [];
}

export async function getFolderTree(folderId: string): Promise<FolderTree> {
  const res = await axios.get(`/api/v1/folders/${folderId}/tree`);
  return res.data as FolderTree;
}

export async function getFolderAncestors(
  folderId: string
): Promise<{ id?: string; name: string }[]> {
  const res = await axios.get(`/api/v1/folders/${folderId}/ancestors`);
  return res.data.ancestors || [];
}

// List trashed folders (primary only) for the current user
export async function listDeletedFolders(
  page = 1,
  limit = 50
): Promise<FolderItem[]> {
  const offset = (page - 1) * limit;
  const params = new URLSearchParams({
    deleted: "true",
    limit: limit.toString(),
    offset: offset.toString(),
  });
  const res = await axios.get<FolderListResponse>(`/api/v1/folders?${params}`);
  return (res.data.folders || []).map((folder) => ({
    ...folder,
    type: "folder" as const,
  }));
}

// Permanently delete a folder
export async function deleteFolderPermanent(
  id: string
): Promise<{ message: string }> {
  const res = await axios.delete(`/api/v1/folders/${id}?permanent=true`);
  return res.data;
}
