import axios from "../lib/axios";

export type FileSearchResult = {
  id: string;
  filename: string;
  mime: string;
  size: number;
  createdAt: string;
  updatedAt: string;
  downloadCount: number;
  contentHash: string;
  physicalSize: number;
  refCount: number;
  dedupSavings: number;
  rank?: number;
};

export type FileSearchResponse = {
  items: FileSearchResult[];
  total: number;
  limit: number;
  offset: number;
};

export type FolderSearchResult = {
  id: string;
  name: string;
  size: number;
  createdAt: string;
  updatedAt: string;
  rank?: number;
};

export type CombinedSearchResponse = {
  files: FileSearchResult[];
  folders: FolderSearchResult[];
  totalFiles: number;
  totalFolders: number;
  limit: number;
  offset: number;
};

export type SearchFilters = {
  q?: string;
  mime?: string;
  minSize?: number;
  maxSize?: number;
  dateFrom?: string;
  dateTo?: string;
  tags?: string[];
  uploader?: string;
  folderId?: string;
  limit?: number;
  offset?: number;
  sort?: string;
};

export async function searchFiles(
  filters: SearchFilters = {}
): Promise<FileSearchResponse> {
  const {
    q,
    mime,
    minSize,
    maxSize,
    dateFrom,
    dateTo,
    tags,
    uploader,
    folderId,
    limit = 50,
    offset = 0,
    sort = "created_at_desc",
  } = filters;

  const body = {
    query: `query SearchFiles(
      $q: String,
      $mime: String,
      $minSize: Int,
      $maxSize: Int,
      $dateFrom: String,
      $dateTo: String,
      $tags: [String!],
      $uploader: String,
      $folderId: ID,
      $limit: Int,
      $offset: Int,
      $sort: String
    ) { 
      searchFiles(
        q: $q,
        mime: $mime,
        minSize: $minSize,
        maxSize: $maxSize,
        dateFrom: $dateFrom,
        dateTo: $dateTo,
        tags: $tags,
        uploader: $uploader,
        folderId: $folderId,
        limit: $limit,
        offset: $offset,
        sort: $sort
      ) { 
        items {
          id
          filename
          mime
          size
          createdAt
          updatedAt
          downloadCount
          contentHash
          physicalSize
          refCount
          dedupSavings
          rank
        }
        total
        limit
        offset
      } 
    }`,
    variables: {
      q,
      mime,
      minSize,
      maxSize,
      dateFrom,
      dateTo,
      tags,
      uploader,
      folderId,
      limit,
      offset,
      sort,
    },
  };

  const res = await axios.post("/api/v1/graphql", body);

  if (res.data.errors) {
    throw new Error(res.data.errors[0]?.message || "GraphQL query failed");
  }

  return res.data.data.searchFiles as FileSearchResponse;
}

// Combined files + folders search via GraphQL searchItems
export async function searchItemsCombined(
  filters: {
    q?: string;
    folderId?: string;
    limit?: number;
    offset?: number;
    sort?: string;
  } = {}
): Promise<CombinedSearchResponse> {
  const {
    q,
    folderId,
    limit = 50,
    offset = 0,
    sort = "created_at_desc",
  } = filters;

  const body = {
    query: `query SearchItems($q: String, $folderId: ID, $limit: Int, $offset: Int, $sort: String) {
      searchItems(q: $q, folderId: $folderId, limit: $limit, offset: $offset, sort: $sort) {
        files {
          id
          filename
          mime
          size
          createdAt
          updatedAt
          downloadCount
          contentHash
          physicalSize
          refCount
          dedupSavings
          rank
        }
        folders {
          id
          name
          size
          createdAt
          updatedAt
          rank
        }
        totalFiles
        totalFolders
        limit
        offset
      }
    }`,
    variables: { q, folderId, limit, offset, sort },
  };

  const res = await axios.post("/api/v1/graphql", body);
  if (res.data.errors) {
    throw new Error(res.data.errors[0]?.message || "GraphQL query failed");
  }
  return res.data.data.searchItems as CombinedSearchResponse;
}

export async function searchFilesLegacy(
  q: string,
  limit = 20
): Promise<{ id: string; name: string; type: "file" | "folder" }[]> {
  const result = await searchFiles({ q, limit });
  return result.items.map((item) => ({
    id: item.id,
    name: item.filename,
    type: "file" as const,
  }));
}
