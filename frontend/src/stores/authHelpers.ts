import { useAuthStore } from "./auth";
import {
  getTokenFromCookie,
  setTokenCookie,
  removeTokenCookie,
} from "../lib/token";

export function getAccessToken(): string | null {
  return getTokenFromCookie();
}

export function setAccessToken(token: string | null) {
  if (token) {
    setTokenCookie(token);
  } else {
    removeTokenCookie();
  }
  useAuthStore.getState().setAccessToken(token);
}
