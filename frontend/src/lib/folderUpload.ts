import {
  initFolderUpload,
  uploadToPresignedUrl,
  completeUpload,
  type FolderInitRequest,
  type FolderInitResponse,
  type UploadProgress,
  computeFileSHA256WithLimit,
} from "@/api/upload";
import { uploadManager } from "./uploadManager";

export type PickedEntry = {
  path: string;
  file: File;
};

export async function pickDirectoryFiles(): Promise<{
  rootName: string;
  entries: PickedEntry[];
}> {
  // Use an input element with webkitdirectory to pick a folder
  return new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    // Set the non-standard attribute for Chromium-based browsers
    (
      input as HTMLInputElement & { webkitdirectory?: boolean }
    ).webkitdirectory = true;
    input.multiple = true;
    input.style.display = "none";
    document.body.appendChild(input);
    input.addEventListener("change", () => {
      const files = Array.from(input.files || []);
      // Determine root folder name from first file's relative path
      let rootName = "Folder";
      if (files.length) {
        type FileWithRelativePath = File & { webkitRelativePath?: string };
        const firstRel = (files[0] as FileWithRelativePath).webkitRelativePath;
        if (firstRel && firstRel.includes("/")) {
          rootName = firstRel.split("/")[0];
        } else if (files[0]) {
          rootName = files[0].name;
        }
      }

      const entries: PickedEntry[] = files.map((f: File) => {
        // Chrome provides a webkitRelativePath with folder path
        type FileWithRelativePath = File & { webkitRelativePath?: string };
        const rel = (f as FileWithRelativePath).webkitRelativePath;
        const path = rel?.replace(/^[^/]*\//, "") || f.name; // strip root folder name
        return { path, file: f };
      });
      document.body.removeChild(input);
      resolve({ rootName, entries });
    });
    input.click();
  });
}

export async function uploadFolder(
  rootName: string,
  entries: PickedEntry[],
  opts?: {
    parentId?: string;
    concurrency?: number;
    onProgress?: (path: string, p: UploadProgress) => void;
  }
): Promise<FolderInitResponse> {
  const filesManifest = await Promise.all(
    entries.map(async ({ path, file }) => {
      const sha = await computeFileSHA256WithLimit(file);
      return { path, size: file.size, mime: file.type, sha256: sha };
    })
  );

  const initReq: FolderInitRequest = {
    parentId: opts?.parentId,
    rootName,
    files: filesManifest,
  };
  const res = await initFolderUpload(initReq);

  // Upload non-dedup files then complete
  const toUpload = res.files.filter((f) => !f.deduped) as Array<
    Extract<(typeof res.files)[number], { deduped: false }>
  >;

  const concurrency = opts?.concurrency ?? 3;
  let index = 0;

  async function worker() {
    while (index < toUpload.length) {
      const current = toUpload[index++];
      const file = entries.find((e) => e.path === current.path)?.file;
      if (!file) continue;
      await uploadToPresignedUrl(file, current.uploadUrl, (p) =>
        opts?.onProgress?.(current.path, p)
      );
      await completeUpload({
        sessionId: current.sessionId,
        clientSha256: filesManifest.find((m) => m.path === current.path)
          ?.sha256,
        folderId: current.targetFolderId,
      });
    }
  }

  const workers = Array.from(
    { length: Math.min(concurrency, toUpload.length) },
    () => worker()
  );
  await Promise.all(workers);

  return res;
}

// Convenience: prepare and enqueue uploads into the global uploadManager so
// they show up in the UploadList toaster like single-file uploads.
export async function enqueueFolderUploads(
  rootName: string,
  entries: PickedEntry[],
  opts?: { parentId?: string }
) {
  const filesManifest = await Promise.all(
    entries.map(async ({ path, file }) => {
      const sha = await computeFileSHA256WithLimit(file);
      return { path, size: file.size, mime: file.type, sha256: sha };
    })
  );

  const res = await initFolderUpload({
    parentId: opts?.parentId,
    rootName,
    files: filesManifest,
  });

  // Enqueue non-dedup files into uploadManager with prepared sessions
  const prepared = res.files
    .filter((f): f is Extract<typeof f, { deduped: false }> => !f.deduped)
    .map((f) => {
      const entry = entries.find((e) => e.path === f.path);
      if (!entry) {
        throw new Error(`Missing file entry for path: ${f.path}`);
      }
      const sha256 = filesManifest.find((m) => m.path === f.path)?.sha256;
      return {
        file: entry.file,
        sessionId: f.sessionId,
        uploadUrl: f.uploadUrl,
        folderId: f.targetFolderId,
        sha256,
      };
    });

  if (prepared.length) {
    uploadManager.addPreparedUploads(prepared);
  }

  return res;
}
