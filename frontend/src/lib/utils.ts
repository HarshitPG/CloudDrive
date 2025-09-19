import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Download a URL (presigned or direct) and save it to the user's device.
// Tries to infer filename from `content-disposition` header if not provided.
export async function downloadUrlToFile(
  url: string,
  filename?: string
): Promise<void> {
  const res = await fetch(url);
  if (!res.ok)
    throw new Error(`Failed to download file: ${res.status} ${res.statusText}`);

  // Try to extract filename from headers when not explicitly provided
  if (!filename) {
    const cd = res.headers.get("content-disposition");
    if (cd) {
      const m = /filename\*=UTF-8''([^;\n]+)|filename="?([^";\n]+)"?/.exec(cd);
      const name = m ? m[1] || m[2] : null;
      if (name) filename = decodeURIComponent(name);
    }
  }

  const blob = await res.blob();
  const blobUrl = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = blobUrl;
  a.download = filename || "download";
  // append to DOM required for Firefox
  document.body.appendChild(a);
  a.click();
  a.remove();
  // free memory after short delay
  setTimeout(() => URL.revokeObjectURL(blobUrl), 1500);
}
