import React, { useState } from "react";
import { forgotPasswordApi } from "../../api/auth";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [loading, setLoading] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setLoading(true);
    try {
      await forgotPasswordApi({ email });
      setMsg("If that account exists, a reset link was sent to the email.");
    } catch (e: unknown) {
      let errorMessage = "request failed";
      if (e instanceof Error) {
        errorMessage = e.message;
      } else if (typeof e === "object" && e !== null && "response" in e) {
        const response = (e as { response?: { data?: { error?: string } } })
          .response;
        errorMessage = response?.data?.error || "request failed";
      }
      setErr(errorMessage);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="max-w-md mx-auto mt-24 p-6 border rounded">
      <h2 className="text-xl font-semibold mb-4">Reset password</h2>
      {msg && <div className="text-green-600 mb-3">{msg}</div>}
      {err && <div className="text-red-600 mb-3">{err}</div>}
      <form onSubmit={submit}>
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
        <button
          type="submit"
          disabled={loading}
          className="px-4 py-2 bg-orange-600 text-white rounded"
        >
          {loading ? "Sending..." : "Send reset link"}
        </button>
      </form>
    </div>
  );
}
