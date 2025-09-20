import {
  createSession,
  completeUpload,
  uploadToPresignedUrl,
  computeFileSHA256WithLimit,
  type UploadProgress,
} from "../api/upload";

export type UploadStatus =
  | "queued"
  | "hashing"
  | "creating"
  | "uploading"
  | "completing"
  | "done"
  | "error"
  | "aborted";

export type UploadItem = {
  id: string;
  file: File;
  status: UploadStatus;
  progress: number;
  loaded: number;
  total: number;
  speedBps?: number;
  error?: string;
  sessionId?: string;
  sha256?: string;
  uploadUrl?: string;
  folderId?: string;
};

export type UploadListener = (items: UploadItem[]) => void;
export type UploadDoneListener = (item: UploadItem) => void;

class UploadManager {
  private items: UploadItem[] = [];
  private listeners: Set<UploadListener> = new Set();
  private doneListeners: Set<UploadDoneListener> = new Set();
  private concurrency = 2;
  private active = 0;
  private abortControllers: Map<string, AbortController> = new Map();

  on(listener: UploadListener) {
    this.listeners.add(listener);
  }
  off(listener: UploadListener) {
    this.listeners.delete(listener);
  }
  onDone(listener: UploadDoneListener) {
    this.doneListeners.add(listener);
  }
  offDone(listener: UploadDoneListener) {
    this.doneListeners.delete(listener);
  }
  private emit() {
    const snapshot = [...this.items];
    this.listeners.forEach((l) => l(snapshot));
  }

  addFiles(files: File[]) {
    const newItems = files.map<UploadItem>((file) => ({
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      file,
      status: "queued",
      progress: 0,
      loaded: 0,
      total: file.size,
    }));
    this.items = [...newItems, ...this.items];
    if (this.items.length > 5) this.items = this.items.slice(0, 5);
    this.emit();
    this.pump();
  }

  // Add pre-initialized uploads (from folder init): already have sessionId, uploadUrl, and folderId
  addPreparedUploads(
    items: Array<{
      file: File;
      sessionId: string;
      uploadUrl: string;
      folderId: string;
      sha256?: string;
    }>
  ) {
    const prepared = items.map<UploadItem>((p) => ({
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      file: p.file,
      status: "queued",
      progress: 0,
      loaded: 0,
      total: p.file.size,
      sessionId: p.sessionId,
      uploadUrl: p.uploadUrl,
      folderId: p.folderId,
      sha256: p.sha256,
    }));
    this.items = [...prepared, ...this.items];
    if (this.items.length > 5) this.items = this.items.slice(0, 5);
    this.emit();
    this.pump();
  }

  getItems() {
    return this.items;
  }

  cancel(id: string) {
    const item = this.items.find((i) => i.id === id);
    if (!item) return;
    const ctrl = this.abortControllers.get(id);
    ctrl?.abort();
    this.updateItem(id, { status: "aborted" });
  }

  remove(id: string) {
    // If actively uploading, abort first
    const ctrl = this.abortControllers.get(id);
    if (ctrl) ctrl.abort();
    this.items = this.items.filter((i) => i.id !== id);
    this.emit();
  }

  retry(id: string) {
    const item = this.items.find((i) => i.id === id);
    if (!item) return;
    this.updateItem(id, {
      status: "queued",
      progress: 0,
      loaded: 0,
      error: undefined,
      sessionId: undefined,
      sha256: undefined,
    });
    this.pump();
  }

  private updateItem(id: string, patch: Partial<UploadItem>) {
    this.items = this.items.map((i) => (i.id === id ? { ...i, ...patch } : i));
    this.emit();
    // Notify done listeners when an item becomes done
    const updated = this.items.find((i) => i.id === id);
    if (updated && updated.status === "done") {
      this.doneListeners.forEach((l) => l(updated));
    }
  }

  private pump() {
    while (this.active < this.concurrency) {
      const next = this.items.find((i) => i.status === "queued");
      if (!next) break;
      this.active++;
      this.process(next).finally(() => {
        this.active--;
        this.pump();
      });
    }
  }

  private async process(item: UploadItem) {
    try {
      // Branch: prepared upload (folder) vs standard single-file flow
      let sha: string | undefined = item.sha256;
      let sessionId: string | undefined = item.sessionId;
      let uploadUrl: string | undefined = item.uploadUrl;

      if (!sessionId || !uploadUrl) {
        // Standard single-file flow: compute SHA and create session
        if (item.file.size > 0) {
          const hashCtrl = new AbortController();
          this.abortControllers.set(item.id, hashCtrl);
          this.updateItem(item.id, { status: "hashing" });
          sha = await computeFileSHA256WithLimit(
            item.file,
            undefined,
            ({ loaded, total }) => {
              const percent = Math.min(99, (loaded / total) * 20);
              this.updateItem(item.id, { progress: percent, loaded });
            },
            hashCtrl.signal
          );
          this.abortControllers.delete(item.id);
        }

        this.updateItem(item.id, { status: "creating" });
        const session = await createSession({
          filename: item.file.name,
          declaredMime: item.file.type,
          originalSize: item.file.size,
          clientSha256: sha,
        });

        if (session.skipUpload) {
          this.updateItem(item.id, {
            status: "done",
            progress: 100,
            loaded: item.total,
          });
          return;
        }
        if (!session.uploadUrl || !session.sessionId) {
          throw new Error("Invalid upload session response");
        }
        sessionId = session.sessionId;
        uploadUrl = session.uploadUrl;
        this.updateItem(item.id, { sessionId });
      }

      const ctrl = new AbortController();
      this.abortControllers.set(item.id, ctrl);

      this.updateItem(item.id, {
        status: "uploading",
        sessionId: sessionId,
      });

      let lastTime = Date.now();
      let lastLoaded = 0;

      await uploadToPresignedUrl(
        item.file,
        uploadUrl!,
        (p: UploadProgress) => {
          const now = Date.now();
          const dt = (now - lastTime) / 1000;
          const dBytes = p.loaded - lastLoaded;
          const speed = dt > 0 ? dBytes / dt : 0;
          lastTime = now;
          lastLoaded = p.loaded;
          this.updateItem(item.id, {
            progress: p.percent,
            loaded: p.loaded,
            speedBps: speed,
          });
        },
        ctrl.signal
      );

      this.updateItem(item.id, { status: "completing" });
      const completed = await completeUpload({
        sessionId: sessionId!,
        clientSha256: sha,
        folderId: item.folderId,
      });
      if (completed?.userFileId) {
        this.updateItem(item.id, {
          status: "done",
          progress: 100,
          loaded: item.total,
        });
      } else {
        this.updateItem(item.id, {
          status: "error",
          error: "Completion failed",
        });
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Upload failed";
      this.updateItem(item.id, { status: "error", error: msg });
    }
  }
}

export const uploadManager = new UploadManager();
