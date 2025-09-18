import React, { useState } from "react";
import { useSearchParams, useNavigate } from "react-router-dom";
import { resetPasswordApi } from "../../api/auth";

export default function ResetPasswordPage() {
  const [params] = useSearchParams();
  const token = params.get("token") || "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const nav = useNavigate();

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    if (!token) {
      setErr("missing token");
      return;
    }
    if (password.length < 8) {
      setErr("password too short");
      return;
    }
    if (password !== confirm) {
      setErr("passwords do not match");
      return;
    }
    setLoading(true);
    try {
      await resetPasswordApi({ token, newPassword: password });
      nav("/login");
    } catch (e: unknown) {
      let errorMessage = "reset failed";
      if (e instanceof Error) {
        errorMessage = e.message;
      } else if (typeof e === "object" && e !== null && "response" in e) {
        const response = (e as { response?: { data?: { error?: string } } })
          .response;
        errorMessage = response?.data?.error || "reset failed";
      }
      setErr(errorMessage);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="max-w-md mx-auto mt-24 p-6 border rounded">
      <h2 className="text-xl font-semibold mb-4">Set new password</h2>
      {err && <div className="text-red-600 mb-3">{err}</div>}
      <form onSubmit={submit}>
        <label className="block mb-2">
          New password
          <input
            type="password"
            required
            minLength={8}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full p-2 border rounded"
          />
        </label>
        <label className="block mb-2">
          Confirm
          <input
            type="password"
            required
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            className="w-full p-2 border rounded"
          />
        </label>
        <button
          type="submit"
          disabled={loading}
          className="px-4 py-2 bg-purple-600 text-white rounded"
        >
          {loading ? "Saving..." : "Save"}
        </button>
      </form>
    </div>
  );
}
