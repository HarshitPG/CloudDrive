import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuthStore } from "../../stores/auth";

export default function SignupPage() {
  const signup = useAuthStore((s) => s.signup);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const nav = useNavigate();

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setLoading(true);
    try {
      await signup(email.trim(), password);
      setMessage("Sign up successful. Check your email for verification link.");
      setTimeout(() => nav("/login"), 2000);
    } catch (e: unknown) {
      let errorMessage = "signup failed";
      if (e instanceof Error) {
        errorMessage = e.message;
      } else if (typeof e === "object" && e !== null && "response" in e) {
        const response = (e as { response?: { data?: { error?: string } } })
          .response;
        errorMessage = response?.data?.error || "signup failed";
      }
      setErr(errorMessage);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="max-w-md mx-auto mt-24 p-6 border rounded">
      <h2 className="text-xl font-semibold mb-4">Create account</h2>
      {message && <div className="mb-3 text-green-600">{message}</div>}
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
            minLength={8}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full p-2 border rounded"
          />
        </label>
        <div className="mt-4">
          <button
            type="submit"
            disabled={loading}
            className="px-4 py-2 bg-green-600 text-white rounded"
          >
            {loading ? "Creating..." : "Create account"}
          </button>
        </div>
      </form>
    </div>
  );
}
