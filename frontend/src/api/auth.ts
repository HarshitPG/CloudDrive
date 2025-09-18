import axios from "../lib/axios";

export type LoginReq = { email: string; password: string };
export type SignupReq = { email: string; password: string };
export type ForgotReq = { email: string };
export type ResetReq = { token: string; newPassword: string };

export async function loginApi(body: LoginReq) {
  const res = await axios.post("/api/v1/auth/login", body);
  return res.data;
}

export async function signupApi(body: SignupReq) {
  const res = await axios.post("/api/v1/auth/signup", body);
  return res.data;
}

export async function verifyApi(token: string) {
  const res = await axios.get("/api/v1/auth/verify", { params: { token } });
  return res.data;
}

export async function forgotPasswordApi(body: ForgotReq) {
  const res = await axios.post("/api/v1/auth/forgot-password", body);
  return res.data;
}

export async function resetPasswordApi(body: ResetReq) {
  const res = await axios.post("/api/v1/auth/reset-password", body);
  return res.data;
}
