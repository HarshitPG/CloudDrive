import { useEffect, useState } from "react";
import { useSearchParams, Link } from "react-router-dom";
import { motion } from "framer-motion";
import { Check } from "lucide-react";
import { verifyApi } from "../../api/auth";

export default function VerifyPage() {
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!token) {
      setErr("Missing verification token.");
      setLoading(false);
      return;
    }

    verifyApi(token)
      .then(() => {
        setMessage("Email verified successfully! You may now sign in.");
      })
      .catch((e) => {
        const errorMessage =
          e?.response?.data?.error ||
          e?.message ||
          "Verification failed. The link may be invalid or expired.";
        setErr(errorMessage);
      })
      .finally(() => setLoading(false));
  }, [token]);

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="drive-card p-6">
        <div className="w-full max-w-md text-center p-8  rounded-lg  bg-white">
          <h2 className="text-2xl font-semibold mb-6 text-gray-800">
            Email Verification
          </h2>

          {loading && (
            <p className="text-gray-500">
              Verifying your email, please wait...
            </p>
          )}

          {err && (
            <p className="text-red-600 bg-red-50 p-3 rounded-md">{err}</p>
          )}

          {message && (
            <div className="flex flex-col items-center gap-4">
              <motion.div
                initial={{ scale: 0 }}
                animate={{ scale: 1 }}
                transition={{ delay: 0.2, type: "spring", stiffness: 180 }}
                className="w-20 h-20 bg-green-500 rounded-full flex items-center justify-center"
              >
                <Check className="w-12 h-12 text-white" />
              </motion.div>

              <div className="mt-2">
                <p className="text-green-700 font-medium">{message}</p>
                <Link
                  to="/login"
                  className="text-blue-600 hover:underline font-semibold"
                >
                  Go to Sign In
                </Link>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
