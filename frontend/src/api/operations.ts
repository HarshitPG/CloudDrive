import axios from "../lib/axios";

export interface DownloadResponse {
  downloadUrl: string;
}

export interface ShareResponse {
  shareId: string;
  token: string;
  url: string;
}

export interface ShareToUserRequest {
  targetUserId: string;
  permission?: string;
}

export interface CreatePublicShareRequest {
  title?: string;
  description?: string;
  expiresAt?: string;
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
    shareData: CreatePublicShareRequest = {}
  ): Promise<ShareResponse> {
    try {
      const response = await axios.post(
        `/api/v1/shares/folders/${folderId}/share`,
        shareData
      );
      return response.data;
    } catch (error) {
      throw new Error(
        `Failed to create public folder share: ${
          error instanceof Error ? error.message : "Unknown error"
        }`
      );
    }
  },
};
