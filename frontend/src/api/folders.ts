import axios from "../lib/axios";

export type FolderItem = {
  id: string;
  name: string;
  type: "folder";
  createdAt: string;
  updatedAt: string;
};

export async function listFolders(parentId?: string): Promise<FolderItem[]> {
  if (!parentId) {
    return [];
  }

  const res = await axios.get(`/api/v1/folders/${parentId}/contents`);
  return res.data.folders || [];
}

export async function createFolder(
  name: string,
  parentId?: string
): Promise<FolderItem> {
  const res = await axios.post("/api/v1/folders", {
    name,
    parent_id: parentId,
  });
  return res.data;
}

export async function deleteFolder(id: string): Promise<void> {
  await axios.delete(`/api/v1/folders/${id}`);
}
