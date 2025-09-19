import { create } from "zustand";
import axios from "axios";

type User = {
  id: string;
  email: string;
} | null;

type AuthState = {
  accessToken: string | null;
  user: User;
  isAuthenticated: boolean;
  setAccessToken: (t: string | null) => void;
  setUser: (u: User) => void;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  signup: (email: string, password: string, fullName: string) => Promise<void>;
  clearAuth: () => void;
};

const authAxios = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || "",
  withCredentials: true,
  timeout: 10_000,
});

export const useAuthStore = create<AuthState>((set) => ({
  accessToken: null,
  user: null,
  isAuthenticated: false,

  setAccessToken: (t: string | null) =>
    set({
      accessToken: t,
      isAuthenticated: !!t,
    }),

  setUser: (u: User) => set({ user: u }),

  login: async (email: string, password: string) => {
    const res = await authAxios.post("/api/v1/auth/login", { email, password });
    const { access_token } = res.data;
    if (!access_token) throw new Error("missing access token from server");
    set({
      accessToken: access_token,
      user: { id: "", email },
      isAuthenticated: true,
    });
  },

  logout: async () => {
    try {
      await authAxios.post("/api/v1/auth/logout");
    } catch {
      //
    } finally {
      set({ accessToken: null, user: null, isAuthenticated: false });
    }
  },

  signup: async (email: string, password: string, fullName: string) => {
    await authAxios.post("/api/v1/auth/signup", { email, password, fullName });
  },

  clearAuth: () => {
    set({ accessToken: null, user: null, isAuthenticated: false });
  },
}));
