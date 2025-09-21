import { useEffect, useState } from "react";
import { fetchQuota, type QuotaResponse } from "../api/quota";

export default function QuotaWidget() {
  const [quota, setQuota] = useState<QuotaResponse | null>(null);

  useEffect(() => {
    let isMounted = true;

    const timer = setTimeout(() => {
      fetchQuota()
        .then((q) => {
          if (isMounted) setQuota(q);
        })
        .catch((error) => {
          console.error("Failed to fetch quota:", error);
          if (isMounted) {
            setQuota({
              original_bytes: 0,
              deduped_bytes: 0,
              quota_bytes: 1,
              savings_bytes: 0,
              savings_percent: 0,
              quota_used_percent: 0,
            });
          }
        });
    }, 200); // Small delay to avoid rate limiting

    return () => {
      isMounted = false;
      clearTimeout(timer);
    };
  }, []);

  if (!quota) {
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
        Storage: {Math.round(quota.original_bytes / 1024 / 1024)}MB /{" "}
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
    </div>
  );
}
