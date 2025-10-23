import React, { useEffect } from "react";
import {
  MessageSquare,
  Loader2,
  AlertCircle,
  Play,
  Trash2,
} from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { useChat } from "@/hooks/useChat";
import { ChatMessageList } from "./ChatMessage";
import { ChatInput } from "./ChatInput";

interface ChatPanelProps {
  fileId: string;
  className?: string;
}

export function ChatPanel({ fileId, className = "" }: ChatPanelProps) {
  const {
    chat,
    isReady,
    isProcessing,
    hasFailed,
    isSending,
    messages,
    processDocument,
    sendMessage,
    loadHistory,
    clearHistory,
  } = useChat(fileId);

  // Load history when ready
  useEffect(() => {
    if (isReady) {
      loadHistory();
    }
  }, [isReady, loadHistory]);

  return (
    <Card className={`flex flex-col h-full ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b border-border">
        <h3 className="text-lg font-semibold flex items-center gap-2">
          <MessageSquare className="w-5 h-5" />
          Chat with Document
        </h3>
        {isReady && messages.length > 0 && (
          <Button
            variant="ghost"
            size="sm"
            onClick={clearHistory}
            disabled={isSending}
            className="text-xs"
          >
            <Trash2 className="w-3 h-3 mr-1" />
            Clear
          </Button>
        )}
      </div>

      {/* Content */}
      <div className="flex-1 flex flex-col min-h-0">
        {/* Not Processed Yet */}
        {!chat || chat.processingStatus === "not_started" ? (
          <div className="flex-1 flex items-center justify-center p-6">
            <div className="text-center max-w-sm space-y-4">
              <MessageSquare className="w-16 h-16 mx-auto text-muted-foreground opacity-50" />
              <div className="space-y-2">
                <h4 className="font-semibold">Enable Document Chat</h4>
                <p className="text-sm text-muted-foreground">
                  Process this document to ask questions and get AI-powered
                  answers based on its content.
                </p>
              </div>
              <Button onClick={processDocument} className="mt-4">
                <Play className="w-4 h-4 mr-2" />
                Process Document
              </Button>
            </div>
          </div>
        ) : null}

        {/* Processing */}
        {isProcessing && (
          <div className="flex-1 flex items-center justify-center p-6">
            <div className="text-center space-y-3">
              <Loader2 className="w-12 h-12 mx-auto animate-spin text-primary" />
              <p className="text-sm text-muted-foreground">
                Indexing document for chat...
              </p>
              <p className="text-xs text-muted-foreground">
                This may take a few moments
              </p>
            </div>
          </div>
        )}

        {/* Failed */}
        {hasFailed && (
          <div className="flex-1 flex items-center justify-center p-6">
            <div className="text-center space-y-3">
              <AlertCircle className="w-12 h-12 mx-auto text-destructive" />
              <p className="text-sm text-destructive">
                {chat?.error || "Failed to process document"}
              </p>
              <Button variant="outline" onClick={processDocument}>
                Try Again
              </Button>
            </div>
          </div>
        )}

        {/* Ready - Show Chat */}
        {isReady && (
          <>
            <ChatMessageList messages={messages} isLoading={isSending} />
            <ChatInput
              onSend={sendMessage}
              disabled={!isReady}
              isLoading={isSending}
            />
          </>
        )}
      </div>
    </Card>
  );
}
