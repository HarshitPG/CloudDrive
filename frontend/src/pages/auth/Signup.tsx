import React, { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuthStore } from "../../stores/auth";
import { motion } from "framer-motion";

// ShadCN UI Components
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

import { Eye, EyeOff, FolderOpen, MailCheck } from "lucide-react";

export default function SignupPage() {
  const signup = useAuthStore((s) => s.signup);
  const navigate = useNavigate();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [fullname, setFullname] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setLoading(true);

    try {
      await signup(email.trim().toLowerCase(), password, fullname.trim());
      setMessage(
        "Sign up successful! Please check your email for a verification link to activate your account."
      );
      setIsModalOpen(true);
    } catch (e: unknown) {
      let errorMessage = "An unknown error occurred during sign-up.";
      if (e instanceof Error) {
        errorMessage = e.message;
      } else if (typeof e === "object" && e !== null && "response" in e) {
        const response = (e as { response?: { data?: { error?: string } } })
          .response;
        errorMessage = response?.data?.error || "Sign-up failed.";
      }
      setErr(errorMessage);
    } finally {
      setLoading(false);
    }
  }

  const handleModalOk = () => {
    setIsModalOpen(false);
    navigate("/login");
  };

  return (
    <>
      <div className="min-h-screen flex items-center justify-center bg-gradient-surface p-4">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }}
          className="w-full max-w-md"
        >
          <Card className="drive-card">
            <CardHeader className="text-center space-y-4">
              <motion.div
                initial={{ scale: 0 }}
                animate={{ scale: 1 }}
                transition={{ delay: 0.2, type: "spring", stiffness: 200 }}
                className="mx-auto w-16 h-16 rounded-2xl flex items-center justify-center shadow-[var(--shadow-glow)]"
                style={{
                  background:
                    "linear-gradient(135deg, hsl(210 100% 56%), hsl(210 100% 70%))",
                }}
              >
                <FolderOpen className="w-12 h-12 text-white" />
              </motion.div>
              <CardTitle className="text-2xl font-bold text-foreground">
                Create an Account
              </CardTitle>
              <CardDescription className="text-muted-foreground">
                Enter your details to get started with CloudDrive.
              </CardDescription>
            </CardHeader>

            <CardContent>
              {err && (
                <div className="mb-4 p-3 bg-destructive/10 text-destructive text-sm font-medium rounded-md text-center">
                  {err}
                </div>
              )}
              <form onSubmit={onSubmit} className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="name">Full Name</Label>
                  <Input
                    id="name"
                    type="text"
                    placeholder="Enter your full name"
                    value={fullname}
                    onChange={(e) => setFullname(e.target.value)}
                    required
                    className="drive-surface"
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="email">Email</Label>
                  <Input
                    id="email"
                    type="email"
                    placeholder="Enter your email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    required
                    className="drive-surface"
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="password">Password</Label>
                  <div className="relative">
                    <Input
                      id="password"
                      type={showPassword ? "text" : "password"}
                      placeholder="Enter your password"
                      value={password}
                      minLength={8}
                      onChange={(e) => setPassword(e.target.value)}
                      required
                      className="drive-surface pr-10"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      className="absolute right-2 top-1/2 -translate-y-1/2 h-8 w-8 p-0"
                      onClick={() => setShowPassword(!showPassword)}
                    >
                      {showPassword ? (
                        <EyeOff className="w-8 h-8" />
                      ) : (
                        <Eye className="w-8 h-8" />
                      )}
                    </Button>
                  </div>
                </div>

                <Button
                  type="submit"
                  className="w-full drive-button-primary"
                  disabled={loading}
                >
                  {loading ? "Creating account..." : "Sign Up"}
                </Button>
              </form>

              <div className="mt-6 text-center">
                <p className="text-sm text-muted-foreground">
                  Already have an account?{" "}
                  <Link
                    to="/login"
                    className="text-primary hover:underline font-medium"
                  >
                    Sign In
                  </Link>
                </p>
              </div>
            </CardContent>
          </Card>
        </motion.div>
      </div>

      <Dialog open={isModalOpen} onOpenChange={setIsModalOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader className="text-center items-center pt-4">
            <div className="h-16 w-16 bg-green-100 rounded-full flex items-center justify-center mb-4">
              <MailCheck className="h-8 w-8 text-green-600" />
            </div>
            <DialogTitle className="text-2xl">Account Created!</DialogTitle>
            <DialogDescription className="pt-2">{message}</DialogDescription>
          </DialogHeader>
          <DialogFooter className="sm:justify-center">
            <Button
              type="button"
              onClick={handleModalOk}
              className="w-full sm:w-auto"
            >
              OK
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
