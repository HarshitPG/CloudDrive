import React from "react";
import { Loader2, FileText, AlertCircle } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { useSummary } from "@/hooks/useSummary";

interface SummaryPanelProps {
  fileId: string;
  className?: string;
}

export function SummaryPanel({ fileId, className = "" }: SummaryPanelProps) {
  const { summary, isLoading, isCompleted, isFailed, startPolling, refresh } =
    useSummary(fileId);

  const StatusPill = ({
    status,
  }: {
    status?: "not_found" | "pending" | "processing" | "completed" | "failed";
  }) => {
    if (!status) return null;
    const base =
      "px-2 py-0.5 rounded-full text-[10px] font-medium border inline-flex items-center gap-1";
    const styles: Record<string, string> = {
      not_found: "bg-muted/40 text-muted-foreground border-transparent",
      pending:
        "bg-yellow-100 text-yellow-800 dark:bg-yellow-800/20 dark:text-yellow-300 border-yellow-200/30",
      processing:
        "bg-blue-100 text-blue-800 dark:bg-blue-800/20 dark:text-blue-300 border-blue-200/30",
      completed:
        "bg-green-100 text-green-800 dark:bg-green-800/20 dark:text-green-300 border-green-200/30",
      failed:
        "bg-red-100 text-red-800 dark:bg-red-800/20 dark:text-red-300 border-red-200/30",
    };
    const label: Record<string, string> = {
      not_found: "Not started",
      pending: "Queued",
      processing: "Processing",
      completed: "Completed",
      failed: "Failed",
    };
    return <span className={`${base} ${styles[status]}`}>{label[status]}</span>;
  };

  // Auto-start polling if we have a pending/processing status
  React.useEffect(() => {
    if (
      summary &&
      (summary.status === "pending" || summary.status === "processing")
    ) {
      startPolling();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [summary?.status]);

  // Render even before summary exists to present consistent layout
  const currentStatus = summary?.status;

  return (
    <Card className={`p-4 lg:p-4 h-full flex flex-col ${className}`}>
      {/* Sticky header */}
      <div className="flex items-center justify-between pb-3 border-b border-border sticky top-0 bg-background z-10">
        <h3 className="text-sm font-semibold flex items-center gap-2">
          <FileText className="w-4 h-4" />
          Summary
        </h3>
        <div className="flex items-center gap-2">
          <StatusPill status={currentStatus} />
          {isCompleted && (
            <Button
              variant="ghost"
              size="sm"
              onClick={refresh}
              className="text-xs"
            >
              Refresh
            </Button>
          )}
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto pt-3 space-y-3">
        {/* Initial hint */}
        {!summary && (
          <div className="text-xs text-muted-foreground">
            Summary will be generated automatically when you download or preview
            the file.
          </div>
        )}

        {/* Loading State */}
        {isLoading && (
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <Loader2 className="w-4 h-4 animate-spin" />
            <span>Generating summary...</span>
          </div>
        )}

        {/* Completed State */}
        {isCompleted && summary?.summary && (
          <div className="prose prose-sm dark:prose-invert max-w-none">
            <p className="text-[13px] leading-relaxed whitespace-pre-wrap">
              {summary.summary}
            </p>
          </div>
        )}

        {/* Failed State */}
        {isFailed && (
          <div className="flex flex-col items-start gap-2">
            <div className="flex items-center gap-2 text-sm text-destructive">
              <AlertCircle className="w-4 h-4" />
              <span>{summary?.error || "Failed to generate summary"}</span>
            </div>
            <Button variant="outline" size="sm" onClick={startPolling}>
              Try Again
            </Button>
          </div>
        )}
      </div>
    </Card>
  );
}
