import axios from "../lib/axios";

// ============ Types ============

// Backend statuses include not_found, pending, processing, completed, failed
export type SummaryStatus =
  | "not_found"
  | "pending"
  | "processing"
  | "completed"
  | "failed";
// Backend statuses: not_started | indexing | ready | failed
export type ChatStatus = "not_started" | "indexing" | "ready" | "failed";

export interface SummaryResult {
  file_id: string;
  status: SummaryStatus;
  summary?: string;
  error?: string;
  created_at: string;
  updated_at: string;
}

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
  timestamp?: string;
}

export interface ChatHistory {
  file_id: string;
  messages: ChatMessage[];
}

export interface ChatResponse {
  answer: string;
  file_id: string;
}

export interface ChatProcessingResult {
  file_id: string;
  status: ChatStatus;
  error?: string;
  created_at: string;
  updated_at: string;
}

// ============ API Functions ============

/**
 * Get summary status/result for a file
 * Polling endpoint - call every 2s while status is pending/processing
 */
export async function getSummary(fileId: string): Promise<SummaryResult> {
  const res = await axios.get(`/api/v1/files/${fileId}/summary`);
  return res.data;
}

/**
 * Start processing a document for chat (indexing)
 * Call this before enabling chat functionality
 */
export async function processForChat(
  fileId: string
): Promise<{ message: string; file_id: string }> {
  const res = await axios.post(`/api/v1/files/${fileId}/process`);
  return res.data;
}

/**
 * Get chat processing status
 * Poll this after processForChat until status is "completed"
 */
export async function getChatStatus(
  fileId: string
): Promise<ChatProcessingResult> {
  const res = await axios.get(`/api/v1/files/${fileId}/chat/status`);
  return res.data;
}

/**
 * Send a chat message/question about the document
 * Only works after document is processed (status = completed)
 */
export async function sendChatMessage(
  fileId: string,
  question: string
): Promise<ChatResponse> {
  const res = await axios.post(
    `/api/v1/files/${fileId}/chat`,
    { question },
    { timeout: 90_000 } // Allow up to 90s for model response
  );
  return res.data;
}

/**
 * Get chat history for a file
 */
export async function getChatHistory(fileId: string): Promise<ChatHistory> {
  const res = await axios.get(`/api/v1/files/${fileId}/chat/history`);
  return res.data;
}

/**
 * Clear chat history for a file
 */
export async function clearChatHistory(
  fileId: string
): Promise<{ message: string }> {
  const res = await axios.delete(`/api/v1/files/${fileId}/chat/history`);
  return res.data;
}
