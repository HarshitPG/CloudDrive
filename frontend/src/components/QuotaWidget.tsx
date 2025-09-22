import { useEffect } from "react";
import { useQuota } from "../contexts/QuotaContext";

export default function QuotaWidget() {
  const { quota, refreshQuota, isLoading } = useQuota();

  useEffect(() => {
    const timer = setTimeout(() => {
      refreshQuota();
    }, 200);

    return () => {
      clearTimeout(timer);
    };
  }, [refreshQuota]);

  if (!quota || isLoading) {
    return (
      <div className="mt-auto">
        <div className="text-sm mb-1">Loading storage info...</div>
        <div className="w-full bg-gray-200 rounded h-2">
          <div className="bg-gray-300 h-2 rounded animate-pulse" />
        </div>
      </div>
    );
  }

  const percent = Math.min(
    100,
    Math.round((quota.original_bytes / quota.quota_bytes) * 100)
  );
  const totalUsed = quota.original_bytes - quota.deduped_bytes;

  return (
    <div className="mt-auto">
      <div className="text-sm mb-1">
        Storage: {Math.round(totalUsed / 1024 / 1024)}MB /{" "}
        {Math.round(quota.quota_bytes / 1024 / 1024)}MB
      </div>
      <div className="w-full bg-gray-200 rounded h-2">
        <div
          className="bg-gray-500 h-2 rounded"
          style={{ width: `${percent}%` }}
        />
      </div>
      {quota.savings_percent > 0 && (
        <div className="text-xs text-gray-500 mt-1">
          {quota.savings_percent.toFixed(1)}% saved by deduplication
        </div>
      )}
      <p className="text-xs text-muted-foreground mt-1">
        Files and folders present in trash also count towards your storage
        quota.
      </p>
    </div>
  );
}
