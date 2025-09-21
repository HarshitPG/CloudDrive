import axios from "../lib/axios";
import type { AxiosProgressEvent } from "axios";

export interface DownloadResponse {
  downloadUrl: string;
}

export interface ShareResponse {
  shareId: string;
  token: string;
  url: string;
}

export interface ShareToUserRequest {
  targetUserEmail: string;
  permission?: string;
}

export interface CreatePublicShareRequest {
  title?: string;
  description?: string;
  expiresAt?: string;
}

// Folder share specific types
export interface CreateFolderShareRequest {
  folderId: string;
  title?: string;
  description?: string;
  recursive?: boolean;
  snapshotMode?: boolean;
  expiresAt?: string;
}

export interface FolderShareResponse {
  shareId: string;
  token: string;
  url: string;
}

export interface FolderShareInfo {
  id: string;
  folderId: string;
  token: string;
  title: string;
  description: string;
  recursive: boolean;
  snapshotMode: boolean;
  expiresAt?: string;
  createdAt: string;
  url: string;
}

export interface SharedFolderItem {
  id: string;
  name: string;
  type: "file" | "folder";
  size?: number;
  mimeType?: string;
  path: string;
  parentId?: string;
  downloadUrl?: string;
}

export interface SharedFolderData {
  share: {
    id: string;
    token: string;
    url: string;
    folderId: string;
    folderName: string;
    title: string;
    description: string;
    recursive: boolean;
    snapshotMode: boolean;
    expiresAt?: string;
    createdAt: string;
  };
  items: SharedFolderItem[];
  total: number;
  hasMore: boolean;
}

export interface ShareUserInfo {
  shareUserId: string;
  userId: string;
  permission: string;
  createdAt: string;
}

export interface PublicShareInfo {
  id: string;
  token: string;
  title: string;
  description: string;
  expiresAt: string;
  createdAt: string;
  url: string;
}

export interface ListSharesResponse {
  userShares: ShareUserInfo[];
  publicShare?: PublicShareInfo;
}

export const fileOperationsApi = {
  /**
   * Download a file - returns presigned URL
   */
  async downloadFile(fileId: string): Promise<DownloadResponse> {
    try {
      const response = await axios.get(`/api/v1/files/${fileId}/download`);
      return response.data;
    } catch (error) {
      throw new Error(
        `Failed to generate download URL: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  /**
   * Delete a file (soft delete)
   */
  async deleteFile(fileId: string): Promise<void> {
    try {
      await axios.delete(`/api/v1/files/${fileId}`);
    } catch (error) {
      throw new Error(
        `Failed to delete file: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  /**
   * Create a public share for a file
   */
  async createPublicFileShare(
    fileId: string,
    shareData: CreatePublicShareRequest = {}
  ): Promise<ShareResponse> {
    try {
      const response = await axios.post(
        `/api/v1/shares/files/${fileId}/share`,
        shareData
      );
      return response.data;
    } catch (error) {
      throw new Error(
        `Failed to create public share: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  //Share a file with a specific user

  async shareFileWithUser(
    fileId: string,
    shareData: ShareToUserRequest
  ): Promise<{ shareId: string }> {
    try {
      const response = await axios.post(
        `/api/v1/shares/files/${fileId}/share/user`,
        shareData
      );
      return response.data;
    } catch (error) {
      throw new Error(
        `Failed to share file with user: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  //List all shares for a file

  async listFileShares(fileId: string): Promise<ListSharesResponse> {
    try {
      const response = await axios.get(`/api/v1/shares/files/${fileId}/shares`);
      return response.data;
    } catch (error) {
      throw new Error(
        `Failed to list file shares: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  //Revoke a share

  async revokeShare(shareId: string): Promise<void> {
    try {
      await axios.delete(`/api/v1/shares/shares/${shareId}`);
    } catch (error) {
      throw new Error(
        `Failed to revoke share: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  // Move a file to a different folder
  async moveFile(fileId: string, targetFolderId: string | null): Promise<void> {
    try {
      await axios.post(`/api/v1/files/${fileId}/move`, {
        targetFolderId: targetFolderId || "",
      });
    } catch (error) {
      throw new Error(
        `Failed to move file: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },
};

// Folder Operations API
export const folderOperationsApi = {
  //Delete a folder (soft delete)

  async deleteFolder(folderId: string): Promise<void> {
    try {
      await axios.delete(`/api/v1/folders/${folderId}`);
    } catch (error) {
      throw new Error(
        `Failed to delete folder: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  // Create a public share for a folder
  async createPublicFolderShare(
    folderId: string,
    shareData: CreatePublicShareRequest
  ): Promise<FolderShareResponse> {
    try {
      const payload: {
        title?: string;
        description?: string;
        expiresAt?: string | null;
      } = {
        title: shareData.title,
        description: shareData.description,
        expiresAt: shareData.expiresAt ? shareData.expiresAt : null,
      };

      const response = await axios.post(
        `/api/v1/shares/folders/${folderId}/share`,
        payload
      );
      return response.data;
    } catch (error) {
      throw new Error(
        `Failed to create folder share: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  // Create a protected folder share (requires authentication)
  async createFolderShare(
    shareData: CreateFolderShareRequest
  ): Promise<FolderShareInfo> {
    try {
      const payload = {
        folderId: shareData.folderId,
        title: shareData.title,
        description: shareData.description,
        recursive: shareData.recursive,
        snapshotMode: shareData.snapshotMode,
        expiresAt: shareData.expiresAt ? shareData.expiresAt : null,
      };

      const response = await axios.post(`/api/v1/folder-shares`, payload);
      return response.data as FolderShareInfo;
    } catch (error) {
      throw new Error(
        `Failed to create protected folder share: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  // Get folder share information
  async getFolderShareInfo(shareId: string): Promise<FolderShareInfo> {
    try {
      const response = await axios.get(`/api/v1/folder-shares/${shareId}`);
      return response.data;
    } catch (error) {
      throw new Error(
        `Failed to get folder share info: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  // Revoke a folder share
  async revokeFolderShare(shareId: string): Promise<void> {
    try {
      await axios.delete(`/api/v1/folder-shares/${shareId}`);
    } catch (error) {
      throw new Error(
        `Failed to revoke folder share: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  // Move a folder under a different parent folder
  async moveFolder(
    folderId: string,
    targetParentId: string | null
  ): Promise<void> {
    try {
      await axios.post(`/api/v1/folders/${folderId}/move`, {
        targetParentId: targetParentId || "",
      });
    } catch (error) {
      throw new Error(
        `Failed to move folder: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  // Download entire folder as a ZIP archive (authenticated)
  async downloadFolderArchive(
    folderId: string,
    recursive = true,
    onProgress?: (downloaded: number, total: number) => void
  ): Promise<Blob> {
    const params = new URLSearchParams();
    if (recursive) params.set("recursive", "true");
    const url = `/api/v1/folders/${folderId}/download?${params.toString()}`;
    try {
      const response = await axios.get(url, {
        responseType: "blob",
        onDownloadProgress: (progressEvent: AxiosProgressEvent) => {
          if (!onProgress || !progressEvent) return;
          const loaded = (progressEvent.loaded as number) || 0;
          const total = (progressEvent.total as number) || 0;
          onProgress(loaded, total);
        },
      });
      return response.data as Blob;
    } catch (error) {
      throw new Error(
        `Failed to download folder archive: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  // Share a folder with a specific user (private share)
  async shareFolderWithUser(
    folderId: string,
    shareData: ShareToUserRequest
  ): Promise<{ shareId: string }> {
    try {
      const response = await axios.post(
        `/api/v1/shares/folders/${folderId}/share/user`,
        shareData
      );
      return response.data;
    } catch (error) {
      throw new Error(
        `Failed to share folder with user: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },
};

// Shared-with-me listing API
export interface SharedWithMeResponse {
  files: Array<{
    id: string;
    filename: string;
    mime: string;
    size: number;
    createdAt: string;
    updatedAt: string;
    downloadCount: number;
  }>;
  folders: Array<{
    id: string;
    name: string;
    createdAt: string;
    updatedAt: string;
  }>;
  limit: number;
  offset: number;
}

export const sharedApi = {
  async listSharedWithMe(
    limit = 50,
    offset = 0
  ): Promise<SharedWithMeResponse> {
    const params = new URLSearchParams({
      limit: String(limit),
      offset: String(offset),
    });
    const response = await axios.get(
      `/api/v1/shares/shared-with-me?${params.toString()}`
    );
    return response.data as SharedWithMeResponse;
  },
  // Attempt to get a download URL or start a download for a shared folder by id
  async downloadFolder(
    folderId: string
  ): Promise<{ downloadUrl?: string } | null> {
    try {
      const response = await axios.get(
        `/api/v1/shares/folders/${folderId}/download`
      );
      return response.data || null;
    } catch (error) {
      throw new Error(
        `Failed to download shared folder: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },
};

// Public share resolution API
export const publicShareApi = {
  // Resolve a shared folder by token
  async resolveFolderShare(
    token: string,
    folderId?: string
  ): Promise<SharedFolderData> {
    try {
      const url = folderId
        ? `/api/v1/fs/${token}?folderId=${folderId}`
        : `/api/v1/fs/${token}`;
      const response = await axios.get(url);
      return response.data;
    } catch (error) {
      throw new Error(
        `Failed to resolve shared folder: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  // Get direct contents for a folder within a public share (lighter-weight)
  async getContents(
    token: string,
    folderId?: string
  ): Promise<SharedFolderData> {
    try {
      const url = folderId
        ? `/api/v1/fs/${token}/contents?folderId=${folderId}`
        : `/api/v1/fs/${token}/contents`;
      const response = await axios.get(url);
      return response.data;
    } catch (error) {
      // fallback to resolveFolderShare
      return this.resolveFolderShare(token, folderId);
    }
  },

  // Get ancestor chain for a folder within a public share
  async getAncestors(
    token: string,
    folderId?: string
  ): Promise<{ id: string; name: string }[]> {
    try {
      const url = folderId
        ? `/api/v1/fs/${token}/ancestors?folderId=${folderId}`
        : `/api/v1/fs/${token}/ancestors`;
      const response = await axios.get(url);
      return response.data.ancestors || [];
    } catch (error) {
      return [];
    }
  },

  // Download a file from a shared folder
  async downloadFromShare(token: string, fileId: string): Promise<string> {
    try {
      const response = await axios.get(
        `/api/v1/fs/${token}/download/${fileId}`
      );
      return response.data.downloadUrl;
    } catch (error) {
      throw new Error(
        `Failed to get download URL: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },

  // Download the entire shared folder as a ZIP archive.
  async downloadFolderArchive(
    token: string,
    onProgress?: (downloaded: number, total: number) => void
  ): Promise<Blob> {
    const url = `/api/v1/fs/${token}/download`;
    try {
      const response = await axios.get(url, {
        responseType: "blob",
        onDownloadProgress: (progressEvent: AxiosProgressEvent) => {
          if (!onProgress || !progressEvent) return;
          const loaded = (progressEvent.loaded as number) || 0;
          const total = (progressEvent.total as number) || 0;
          onProgress(loaded, total);
        },
      });

      return response.data as Blob;
    } catch (error) {
      throw new Error(
        `Failed to download folder archive: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },
  // Resolve a public share file
  async resolveShare(
    token: string
  ): Promise<
    | ({ type: "file" } & ResolvedPublicFile)
    | ({ type: "folder" } & ResolvedPublicFolder)
  > {
    try {
      const response = await axios.get(`/api/v1/s/${token}`);
      return response.data;
    } catch (error) {
      throw new Error(
        `Failed to resolve share: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },
};

export interface ResolvedPublicFile {
  fileId: string;
  filename: string;
  size?: number;
  download: string;
}

export interface ResolvedPublicFolder {
  files: Array<{
    id: string;
    filename: string;
    size?: number;
    download?: string;
  }>;
}
