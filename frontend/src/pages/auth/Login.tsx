import React, { useState } from "react";
import { useNavigate, Link } from "react-router-dom";
import { useAuthStore } from "../../stores/auth";

export default function LoginPage() {
  const login = useAuthStore((s) => s.login);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const nav = useNavigate();

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setLoading(true);
    try {
      await login(email.trim(), password);
      nav("/");
    } catch (e: unknown) {
      let errorMessage = "login failed";
      if (e instanceof Error) {
        errorMessage = e.message;
      } else if (typeof e === "object" && e !== null && "response" in e) {
        const response = (e as { response?: { data?: { error?: string } } })
          .response;
        errorMessage = response?.data?.error || "login failed";
      }
      setErr(errorMessage);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="max-w-md mx-auto mt-24 p-6 border rounded">
      <h2 className="text-xl font-semibold mb-4">Sign in</h2>
      {err && <div className="mb-3 text-red-600">{err}</div>}
      <form onSubmit={onSubmit}>
        <label className="block mb-2">
          Email
          <input
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="w-full p-2 border rounded"
          />
        </label>
        <label className="block mb-2">
          Password
          <input
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full p-2 border rounded"
          />
        </label>
        <div className="flex justify-between items-center mt-4">
          <button
            type="submit"
            disabled={loading}
            className="px-4 py-2 bg-blue-600 text-white rounded"
          >
            {loading ? "Signing..." : "Sign in"}
          </button>
          <Link to="/forgot-password" className="text-sm text-blue-600">
            Forgot?
          </Link>
        </div>
      </form>
      <div className="mt-4 text-sm">
        New?{" "}
        <Link to="/signup" className="text-blue-600">
          Create an account
        </Link>
      </div>
    </div>
  );
}
