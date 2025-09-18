import { useAuthStore } from "./auth";

export function getAccessToken(): string | null {
  return useAuthStore.getState().accessToken;
}

export function setAccessToken(token: string | null) {
  useAuthStore.getState().setAccessToken(token);
}
