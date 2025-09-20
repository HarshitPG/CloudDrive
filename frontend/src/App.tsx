import { Routes, Route, Navigate } from "react-router-dom";
import LoginPage from "./pages/auth/Login";
import SignupPage from "./pages/auth/Signup";
import VerifyPage from "./pages/auth/Verify";
import ForgotPasswordPage from "./pages/auth/ForgotPassword";
import ResetPasswordPage from "./pages/auth/ResetPassword";
import HomePage from "./pages/drive/Dashboard";
import SharedView from "./pages/drive/SharedView";
import PublicShareView from "./pages/drive/PublicShareView";
import PublicShareResolver from "./pages/drive/PublicShareResolver";
import RecentView from "./pages/drive/RecentView";
import StarredView from "./pages/drive/StarredView";
import TrashView from "./pages/drive/TrashView";
import ProtectedRoute from "./components/ProtectedRoute";
import DashboardLayout from "./components/layout/DashboardLayout";

export default function App() {
  return (
    <Routes>
      <Route
        path="/dashboard"
        element={
          <ProtectedRoute>
            <DashboardLayout />
          </ProtectedRoute>
        }
      >
        <Route path="home" element={<HomePage />} />
        <Route path="home/folder/:folderId" element={<HomePage />} />
        <Route path="shared" element={<SharedView />} />
        <Route path="recent" element={<RecentView />} />
        <Route path="starred" element={<StarredView />} />
        <Route path="trash" element={<TrashView />} />
      </Route>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<LoginPage />} />
      <Route path="/signup" element={<SignupPage />} />
      <Route path="/verify-email" element={<VerifyPage />} />
      <Route path="/forgot-password" element={<ForgotPasswordPage />} />
      <Route path="/reset-password" element={<ResetPasswordPage />} />
      <Route path="/fs/:token" element={<PublicShareView />} />
      <Route path="/s/:token" element={<PublicShareResolver />} />
      <Route path="*" element={<Navigate to="/" />} />
    </Routes>
  );
}
