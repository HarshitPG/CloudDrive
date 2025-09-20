export const getFileKind = (mime?: string, filename?: string) => {
  if (!mime && filename) {
    const ext = filename.split(".").pop()?.toLowerCase() || "";
    if (/(jpg|jpeg|png|gif|webp|svg|bmp)/.test(ext)) return "image";
    if (/(mp4|webm|mov|mkv)/.test(ext)) return "video";
    if (/(mp3|wav|ogg|flac)/.test(ext)) return "audio";
    if (/(pdf)/.test(ext)) return "pdf";
    if (/(txt|md|json|xml|csv|log)/.test(ext)) return "text";
    return "other";
  }
  if (!mime) return "other";
  if (mime.startsWith("image/")) return "image";
  if (mime.startsWith("video/")) return "video";
  if (mime.startsWith("audio/")) return "audio";
  if (mime === "application/pdf") return "pdf";
  if (mime.startsWith("text/")) return "text";
  return "other";
};
