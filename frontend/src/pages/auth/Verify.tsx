import { useEffect, useState } from "react";
import { useSearchParams, Link } from "react-router-dom";
import { verifyApi } from "../../api/auth";

export default function VerifyPage() {
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!token) {
      setErr("missing token");
      return;
    }
    setLoading(true);
    verifyApi(token)
      .then(() => {
        setMessage("Email verified. You may now log in.");
      })
      .catch((e) => {
        setErr(e?.response?.data?.error || e?.message || "verify failed");
      })
      .finally(() => setLoading(false));
  }, [token]);

  return (
    <div className="max-w-md mx-auto mt-24 p-6 border rounded">
      <h2 className="text-xl font-semibold mb-4">Verify email</h2>
      {loading && <div>Verifying…</div>}
      {message && (
        <div className="text-green-600">
          {message}{" "}
          <Link to="/login" className="text-blue-600">
            Sign in
          </Link>
        </div>
      )}
      {err && <div className="text-red-600">{err}</div>}
    </div>
  );
}
