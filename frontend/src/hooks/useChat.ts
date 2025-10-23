import { useEffect, useCallback, useRef, useState } from "react";
import { useLLMStore } from "../stores/llm";
import * as llmApi from "../api/llm";
import type { ChatMessage, ChatProcessingResult } from "../api/llm";

const POLLING_INTERVAL = 2000; // 2 seconds
const MAX_RETRIES = 3;

export function useChat(fileId: string | undefined) {
  const {
    getChat,
    setChat,
    addChatMessage,
    setChatMessages,
    setChatProcessing,
    startPolling,
    stopPolling,
  } = useLLMStore();

  const [isSending, setIsSending] = useState(false);
  const retryCountRef = useRef(0);
  const chat = fileId ? getChat(fileId) : undefined;

  /**
   * Initialize chat state for a file
   */
  const initializeChat = useCallback(() => {
    if (!fileId) return;

    setChat(fileId, {
      processingStatus: "not_started",
      messages: [],
      isProcessing: false,
    });
  }, [fileId, setChat]);

  /**
   * Fetch chat processing status
   */
  const fetchChatStatus = useCallback(async () => {
    if (!fileId) return;

    try {
      const result: ChatProcessingResult = await llmApi.getChatStatus(fileId);

      setChat(fileId, {
        processingStatus: result.status,
        error: result.error,
      });

      retryCountRef.current = 0;

      // Stop polling if ready or failed
      if (result.status === "ready" || result.status === "failed") {
        stopPolling(`chat-status-${fileId}`);
      }
    } catch (error) {
      console.error("Failed to fetch chat status:", error);

      retryCountRef.current++;

      if (retryCountRef.current >= MAX_RETRIES) {
        setChat(fileId, {
          processingStatus: "failed",
          error: "Failed to check processing status",
        });
        stopPolling(`chat-status-${fileId}`);
      }
    }
  }, [fileId, setChat, stopPolling]);

  /**
   * Start document processing for chat
   */
  const processDocument = useCallback(async () => {
    if (!fileId) return;

    try {
      await llmApi.processForChat(fileId);

      setChat(fileId, {
        processingStatus: "indexing",
        error: undefined,
      });

      // Start polling for status
      const interval = setInterval(fetchChatStatus, POLLING_INTERVAL);
      startPolling(`chat-status-${fileId}`, interval);

      // Also fetch immediately
      fetchChatStatus();
    } catch (error) {
      console.error("Failed to process document:", error);
      setChat(fileId, {
        processingStatus: "failed",
        error: "Failed to start document processing",
      });
    }
  }, [fileId, setChat, fetchChatStatus, startPolling]);

  /**
   * Send a chat message
   */
  const sendMessage = useCallback(
    async (question: string) => {
      if (!fileId || !question.trim()) return;

      const userMessage: ChatMessage = {
        role: "user",
        content: question.trim(),
        timestamp: new Date().toISOString(),
      };

      // Add user message immediately
      addChatMessage(fileId, userMessage);
      setIsSending(true);
      setChatProcessing(fileId, true);

      try {
        const response = await llmApi.sendChatMessage(fileId, question.trim());

        const assistantMessage: ChatMessage = {
          role: "assistant",
          content: response.answer,
          timestamp: new Date().toISOString(),
        };

        addChatMessage(fileId, assistantMessage);
      } catch (error) {
        console.error("Failed to send chat message:", error);

        // Add error message
        const errorMessage: ChatMessage = {
          role: "assistant",
          content: "Sorry, I couldn't process your question. Please try again.",
          timestamp: new Date().toISOString(),
        };
        addChatMessage(fileId, errorMessage);
      } finally {
        setIsSending(false);
        setChatProcessing(fileId, false);
      }
    },
    [fileId, addChatMessage, setChatProcessing]
  );

  /**
   * Load chat history
   */
  const loadHistory = useCallback(async () => {
    if (!fileId) return;

    try {
      const history = await llmApi.getChatHistory(fileId);
      setChatMessages(fileId, history.messages || []);
    } catch (error) {
      console.error("Failed to load chat history:", error);
    }
  }, [fileId, setChatMessages]);

  /**
   * Clear chat history
   */
  const clearHistory = useCallback(async () => {
    if (!fileId) return;

    try {
      await llmApi.clearChatHistory(fileId);
      setChatMessages(fileId, []);
    } catch (error) {
      console.error("Failed to clear chat history:", error);
    }
  }, [fileId, setChatMessages]);

  /**
   * Check if document is ready for chat
   */
  const isReady = chat?.processingStatus === "ready";

  /**
   * Check if document is being processed
   */
  const isProcessing = chat?.processingStatus === "indexing";

  /**
   * Check if processing failed
   */
  const hasFailed = chat?.processingStatus === "failed";

  // Initialize chat state when fileId changes
  useEffect(() => {
    if (fileId && !chat) {
      initializeChat();
    }
  }, [fileId, chat, initializeChat]);

  // Always fetch current status on mount/file change
  useEffect(() => {
    if (!fileId) return;
    fetchChatStatus();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fileId]);

  // Auto-manage polling when status is indexing
  useEffect(() => {
    if (!fileId) return;
    if (chat?.processingStatus === "indexing") {
      const interval = setInterval(fetchChatStatus, POLLING_INTERVAL);
      startPolling(`chat-status-${fileId}`, interval);
      return () => stopPolling(`chat-status-${fileId}`);
    }
    if (
      chat?.processingStatus === "ready" ||
      chat?.processingStatus === "failed"
    ) {
      stopPolling(`chat-status-${fileId}`);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fileId, chat?.processingStatus]);

  return {
    chat,
    isReady,
    isProcessing,
    hasFailed,
    isSending,
    messages: chat?.messages || [],
    processDocument,
    sendMessage,
    loadHistory,
    clearHistory,
  };
}
