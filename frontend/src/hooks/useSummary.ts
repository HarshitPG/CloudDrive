import { useEffect, useCallback, useRef } from "react";
import { useLLMStore } from "../stores/llm";
import * as llmApi from "../api/llm";
import type { SummaryResult } from "../api/llm";

const POLLING_INTERVAL = 2000; // 2 seconds
// Keep polling continuously until status is completed or failed

export function useSummary(fileId: string | undefined) {
  const { getSummary, setSummary, startPolling, stopPolling } = useLLMStore();

  const retryCountRef = useRef(0);
  const summary = fileId ? getSummary(fileId) : undefined;

  /**
   * Fetch summary status from API
   */
  const fetchSummary = useCallback(async () => {
    if (!fileId) return;

    try {
      const result: SummaryResult = await llmApi.getSummary(fileId);

      setSummary(fileId, {
        status: result.status,
        summary: result.summary,
        error: result.error,
      });

      retryCountRef.current = 0; // Reset on success

      // Stop polling if completed or failed
      if (result.status === "completed" || result.status === "failed") {
        stopPolling(`summary-${fileId}`);
      }
    } catch (error: unknown) {
      console.error("Failed to fetch summary:", error);

      // On errors, keep polling; don't mark as failed here
      retryCountRef.current++;
    }
  }, [fileId, setSummary, stopPolling]);

  /**
   * Start polling for summary updates
   */
  const startSummaryPolling = useCallback(() => {
    if (!fileId) return;

    // Fetch immediately
    fetchSummary();

    // Start interval polling
    const interval = setInterval(fetchSummary, POLLING_INTERVAL);
    startPolling(`summary-${fileId}`, interval);
  }, [fileId, fetchSummary, startPolling]);

  /**
   * Stop polling
   */
  const stopSummaryPolling = useCallback(() => {
    if (!fileId) return;
    stopPolling(`summary-${fileId}`);
  }, [fileId, stopPolling]);

  /**
   * Refresh summary (manual fetch)
   */
  const refreshSummary = useCallback(() => {
    fetchSummary();
  }, [fetchSummary]);

  // Always start polling when fileId is present; stop on unmount or file change
  useEffect(() => {
    if (!fileId) return;
    startSummaryPolling();
    return () => {
      stopSummaryPolling();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fileId]);

  return {
    summary,
    isLoading:
      summary?.status === "pending" || summary?.status === "processing",
    isCompleted: summary?.status === "completed",
    isFailed: summary?.status === "failed",
    startPolling: startSummaryPolling,
    stopPolling: stopSummaryPolling,
    refresh: refreshSummary,
  };
}
