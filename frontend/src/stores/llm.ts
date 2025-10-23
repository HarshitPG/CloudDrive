import { create } from "zustand";
import type {
  SummaryResult,
  ChatMessage,
  ChatProcessingResult,
} from "../api/llm";

// ============ Types ============

export interface FileSummaryState {
  status: SummaryResult["status"];
  summary?: string;
  error?: string;
  lastFetched?: number;
}

export interface FileChatState {
  processingStatus: ChatProcessingResult["status"]; // not_started | indexing | ready | failed
  messages: ChatMessage[];
  isProcessing: boolean; // Is currently processing a chat message
  error?: string;
  lastFetched?: number;
}

interface LLMState {
  // Summary state per file
  summaries: Record<string, FileSummaryState>;

  // Chat state per file
  chats: Record<string, FileChatState>;

  // Active polling intervals
  pollingIntervals: Record<string, NodeJS.Timeout>;

  // ============ Summary Actions ============

  /**
   * Set summary state for a file
   */
  setSummary: (fileId: string, summary: Partial<FileSummaryState>) => void;

  /**
   * Clear summary for a file
   */
  clearSummary: (fileId: string) => void;

  /**
   * Get summary state for a file
   */
  getSummary: (fileId: string) => FileSummaryState | undefined;

  // ============ Chat Actions ============

  /**
   * Set chat state for a file
   */
  setChat: (fileId: string, chat: Partial<FileChatState>) => void;

  /**
   * Add a message to chat history
   */
  addChatMessage: (fileId: string, message: ChatMessage) => void;

  /**
   * Set all chat messages for a file
   */
  setChatMessages: (fileId: string, messages: ChatMessage[]) => void;

  /**
   * Clear chat history for a file
   */
  clearChat: (fileId: string) => void;

  /**
   * Get chat state for a file
   */
  getChat: (fileId: string) => FileChatState | undefined;

  /**
   * Set chat processing status
   */
  setChatProcessing: (fileId: string, isProcessing: boolean) => void;

  // ============ Polling Management ============

  /**
   * Start polling interval for a file (summary or chat status)
   */
  startPolling: (key: string, interval: NodeJS.Timeout) => void;

  /**
   * Stop polling interval for a file
   */
  stopPolling: (key: string) => void;

  /**
   * Stop all polling intervals
   */
  stopAllPolling: () => void;

  // ============ Cleanup ============

  /**
   * Clear all state for a file
   */
  clearFileState: (fileId: string) => void;

  /**
   * Reset entire store
   */
  reset: () => void;
}

// ============ Store Implementation ============

export const useLLMStore = create<LLMState>((set, get) => ({
  summaries: {},
  chats: {},
  pollingIntervals: {},

  // Summary Actions
  setSummary: (fileId, summary) =>
    set((state) => ({
      summaries: {
        ...state.summaries,
        [fileId]: {
          ...state.summaries[fileId],
          ...summary,
          lastFetched: Date.now(),
        },
      },
    })),

  clearSummary: (fileId) =>
    set((state) => {
      const { [fileId]: _, ...rest } = state.summaries;
      return { summaries: rest };
    }),

  getSummary: (fileId) => get().summaries[fileId],

  // Chat Actions
  setChat: (fileId, chat) =>
    set((state) => ({
      chats: {
        ...state.chats,
        [fileId]: {
          messages: [],
          processingStatus: "not_started",
          isProcessing: false,
          ...state.chats[fileId],
          ...chat,
          lastFetched: Date.now(),
        },
      },
    })),

  addChatMessage: (fileId, message) =>
    set((state) => {
      const currentChat = state.chats[fileId] || {
        messages: [],
        processingStatus: "not_started" as const,
        isProcessing: false,
      };

      return {
        chats: {
          ...state.chats,
          [fileId]: {
            ...currentChat,
            messages: [...currentChat.messages, message],
            lastFetched: Date.now(),
          },
        },
      };
    }),

  setChatMessages: (fileId, messages) =>
    set((state) => ({
      chats: {
        ...state.chats,
        [fileId]: {
          ...state.chats[fileId],
          messages,
          lastFetched: Date.now(),
        },
      },
    })),

  clearChat: (fileId) =>
    set((state) => {
      const { [fileId]: _, ...rest } = state.chats;
      return { chats: rest };
    }),

  getChat: (fileId) => get().chats[fileId],

  setChatProcessing: (fileId, isProcessing) =>
    set((state) => ({
      chats: {
        ...state.chats,
        [fileId]: {
          ...state.chats[fileId],
          isProcessing,
        },
      },
    })),

  // Polling Management
  startPolling: (key, interval) =>
    set((state) => {
      // Clear existing interval if any
      if (state.pollingIntervals[key]) {
        clearInterval(state.pollingIntervals[key]);
      }

      return {
        pollingIntervals: {
          ...state.pollingIntervals,
          [key]: interval,
        },
      };
    }),

  stopPolling: (key) =>
    set((state) => {
      const interval = state.pollingIntervals[key];
      if (interval) {
        clearInterval(interval);
      }

      const { [key]: _, ...rest } = state.pollingIntervals;
      return { pollingIntervals: rest };
    }),

  stopAllPolling: () => {
    const intervals = get().pollingIntervals;
    Object.values(intervals).forEach((interval) => clearInterval(interval));
    set({ pollingIntervals: {} });
  },

  // Cleanup
  clearFileState: (fileId) => {
    get().clearSummary(fileId);
    get().clearChat(fileId);
    get().stopPolling(`summary-${fileId}`);
    get().stopPolling(`chat-status-${fileId}`);
  },

  reset: () => {
    get().stopAllPolling();
    set({
      summaries: {},
      chats: {},
      pollingIntervals: {},
    });
  },
}));
