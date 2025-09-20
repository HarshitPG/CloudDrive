import axios, {
  AxiosError,
  type AxiosInstance,
  type InternalAxiosRequestConfig,
} from "axios";
import { getAccessToken, setAccessToken } from "../stores/authHelpers";
import { useAuthStore } from "../stores/auth";

const API_BASE = import.meta.env.VITE_API_BASE || "";

const instance: AxiosInstance = axios.create({
  baseURL: API_BASE,
  withCredentials: true,
  timeout: 15_000,
});

let refreshPromise: Promise<string | null> | null = null;
async function refreshToken(): Promise<string | null> {
  if (!refreshPromise) {
    refreshPromise = (async () => {
      let token: string | null = null;
      try {
        const refreshAxios = axios.create({
          baseURL: API_BASE,
          withCredentials: true,
          timeout: 10_000,
        });

        const res = await refreshAxios.post("/api/v1/auth/refresh");
        const accessToken = res.data?.access_token;

        if (accessToken) {
          setAccessToken(accessToken);
          token = accessToken;
        }
      } catch {
        //
      } finally {
        if (!getAccessToken()) {
          useAuthStore.getState().clearAuth();
        }
      }
      const result = token;
      refreshPromise = null;
      return result;
    })();
  }
  return refreshPromise;
}

instance.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = getAccessToken();
    if (token && config.headers) {
      config.headers["Authorization"] = `Bearer ${token}`;
    }
    return config;
  },
  (error) => Promise.reject(error)
);

instance.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & {
      _retry?: boolean;
    };

    if (
      error.response?.status === 401 &&
      originalRequest &&
      !originalRequest._retry
    ) {
      originalRequest._retry = true;

      const newToken = await refreshToken();

      if (newToken && originalRequest.headers) {
        originalRequest.headers["Authorization"] = `Bearer ${newToken}`;
        return instance(originalRequest);
      }
    }

    return Promise.reject(error);
  }
);

export default instance;
