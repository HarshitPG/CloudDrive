import axios from "../lib/axios";

export type CreateSessionRequest = {
  filename: string;
  declaredMime?: string;
  originalSize: number;
  clientSha256?: string;
};

export type CreateSessionResponse = {
  sessionId?: string;
  uploadUrl?: string;
  tempBlobKey?: string;
  skipUpload: boolean;
  existingFileId?: string;
  userFileId?: string;
};

export type CompleteRequest = {
  sessionId: string;
  clientSha256?: string;
  folderId?: string;
};

export type CompleteResponse = {
  userFileId: string;
  contentId?: string;
  deduped: boolean;
};

export type FolderInitFile = {
  path: string;
  size: number;
  mime?: string;
  sha256?: string;
};

export type FolderInitRequest = {
  parentId?: string;
  rootName: string;
  files: FolderInitFile[];
  idempotencyKey?: string;
};

export type FolderInitFileResponse =
  | { path: string; deduped: true; userFileId: string }
  | {
      path: string;
      deduped: false;
      sessionId: string;
      uploadUrl: string;
      tempBlobKey: string;
      targetFolderId: string;
    };

export type FolderInitResponse = {
  uploadId: string;
  rootFolderId: string;
  folders: Array<{ path: string; folderId: string }>;
  files: FolderInitFileResponse[];
};

export class QuotaExceededError extends Error {
  usedBytes: number;
  quotaBytes: number;
  attempt: number;
  constructor(
    message: string,
    usedBytes: number,
    quotaBytes: number,
    attempt: number
  ) {
    super(message);
    this.name = "QuotaExceededError";
    this.usedBytes = usedBytes;
    this.quotaBytes = quotaBytes;
    this.attempt = attempt;
  }
}

export async function createSession(
  req: CreateSessionRequest
): Promise<CreateSessionResponse> {
  try {
    const res = await axios.post("/api/v1/uploads/session", req);
    return res.data as CreateSessionResponse;
  } catch (err: unknown) {
    const anyErr = err as {
      response?: {
        data?: {
          error?: string;
          usedBytes?: number;
          quotaBytes?: number;
          attempt?: number;
        };
      };
    };
    const data = anyErr?.response?.data;
    if (data && data.error === "quota exceeded") {
      throw new QuotaExceededError(
        data.error,
        data.usedBytes ?? 0,
        data.quotaBytes ?? 0,
        data.attempt ?? req.originalSize
      );
    }
    throw err;
  }
}

export async function completeUpload(
  req: CompleteRequest
): Promise<CompleteResponse> {
  const res = await axios.post("/api/v1/uploads/complete", req);
  return res.data as CompleteResponse;
}

export async function initFolderUpload(
  req: FolderInitRequest
): Promise<FolderInitResponse> {
  const res = await axios.post("/api/v1/uploads/folder/init", req);
  return res.data as FolderInitResponse;
}

export async function abortUpload(sessionId: string): Promise<void> {
  await axios.post("/api/v1/uploads/abort", { sessionId });
}

export type UploadProgress = {
  loaded: number;
  total: number;
  percent: number;
  speedBps?: number;
};

export function uploadToPresignedUrl(
  file: File,
  url: string,
  onProgress?: (p: UploadProgress) => void,
  signal?: AbortSignal
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", url);
    xhr.responseType = "text";

    // Avoid sending auth headers
    xhr.withCredentials = false;

    xhr.upload.onprogress = (e) => {
      if (!e.lengthComputable) return;
      const percent = (e.loaded / e.total) * 100;
      onProgress?.({ loaded: e.loaded, total: e.total, percent });
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        onProgress?.({ loaded: file.size, total: file.size, percent: 100 });
        resolve();
      } else {
        reject(new Error(`Upload failed with status ${xhr.status}`));
      }
    };

    xhr.onerror = () => reject(new Error("Network error during upload"));
    xhr.onabort = () => reject(new Error("Upload aborted"));

    if (signal) {
      const onAbort = () => {
        try {
          xhr.abort();
        } catch (e) {
          //
        }
      };
      if (signal.aborted) {
        onAbort();
        return;
      }
      signal.addEventListener("abort", onAbort, { once: true });
    }

    const contentType = file.type || "application/octet-stream";
    xhr.setRequestHeader("Content-Type", contentType);
    xhr.send(file);
  });
}

export async function computeFileSHA256(
  file: File,
  onProgress?: (p: { loaded: number; total: number }) => void,
  signal?: AbortSignal
): Promise<string> {
  // This implementation reads the file stream and buffers chunks into memory
  // then computes a SHA-256 over the whole concatenated buffer. It's fine for
  // small files but will OOM for very large files. Use computeFileSHA256WithLimit
  // to guard against large inputs or replace with a worker-based streaming hasher.
  if (signal?.aborted) throw new Error("Hashing aborted");

  const total = file.size;
  let loaded = 0;
  const chunks: ArrayBuffer[] = [];

  const reader = file.stream().getReader();
  try {
    while (true) {
      if (signal?.aborted) throw new Error("Hashing aborted");
      const { done, value } = await reader.read();
      if (done) break;
      const buf = value.buffer as ArrayBuffer;
      chunks.push(buf);
      loaded += value.byteLength;
      onProgress?.({ loaded, total });
    }
  } finally {
    try {
      reader.releaseLock();
    } catch (e) {
      // ignore
    }
  }

  const concatenated = concatenateArrayBuffers(chunks);
  const hashBuffer = await crypto.subtle.digest("SHA-256", concatenated);
  return bufferToHex(hashBuffer);
}

/**
 * Compute SHA-256 only for files up to `maxBytes`. Returns undefined if file
 * is larger than `maxBytes` to signal caller to skip client-side hashing.
 */
export async function computeFileSHA256WithLimit(
  file: File,
  maxBytes = 32 * 1024 * 1024,
  onProgress?: (p: { loaded: number; total: number }) => void,
  signal?: AbortSignal
): Promise<string | undefined> {
  if (file.size > maxBytes) return undefined;
  return computeFileSHA256(file, onProgress, signal);
}

function concatenateArrayBuffers(buffers: ArrayBuffer[]): ArrayBuffer {
  const totalLength = buffers.reduce((sum, b) => sum + b.byteLength, 0);
  const tmp = new Uint8Array(totalLength);
  let offset = 0;
  for (const b of buffers) {
    tmp.set(new Uint8Array(b), offset);
    offset += b.byteLength;
  }
  return tmp.buffer;
}

function bufferToHex(buffer: ArrayBuffer): string {
  const byteArray = new Uint8Array(buffer);
  const hexCodes: string[] = [];
  for (const value of byteArray) {
    const hex = value.toString(16).padStart(2, "0");
    hexCodes.push(hex);
  }
  return hexCodes.join("");
}
