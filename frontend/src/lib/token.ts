const TOKEN_COOKIE = "access_token";
const DEFAULT_MAX_AGE = 10 * 60 * 60;
export function getTokenFromCookie(): string | null {
  if (typeof document === "undefined") return null;
  const match = document.cookie
    .split(";")
    .map((c) => c.trim())
    .find((c) => c.startsWith(`${TOKEN_COOKIE}=`));
  if (!match) return null;
  try {
    return decodeURIComponent(match.split("=")[1] || "");
  } catch {
    return null;
  }
}

export function setTokenCookie(
  token: string,
  maxAgeSeconds: number = DEFAULT_MAX_AGE
) {
  if (typeof document === "undefined") return;
  document.cookie = `${TOKEN_COOKIE}=${encodeURIComponent(
    token
  )}; Path=/; Max-Age=${maxAgeSeconds}; SameSite=Lax`;
}

export function removeTokenCookie() {
  if (typeof document === "undefined") return;
  document.cookie = `${TOKEN_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax`;
}

export function getTokenCookieName() {
  return TOKEN_COOKIE;
}
