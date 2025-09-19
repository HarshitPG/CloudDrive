import axios from "../lib/axios";

export type QuotaResponse = {
  original_bytes: number;
  deduped_bytes: number;
  quota_bytes: number;
  savings_bytes: number;
  savings_percent: number;
  quota_used_percent: number;
};

export async function fetchQuota(): Promise<QuotaResponse> {
  const res = await axios.get("/api/v1/users/me/usage");
  return res.data;
}
