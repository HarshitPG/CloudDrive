import { Outlet } from "react-router-dom";
import { useAuthStore } from "../../stores/auth";
import QuotaWidget from "../QuotaWidget";
import { UploadList } from "@/components/upload/UploadList";
import SidebarToggle from "../ui/SidebarToggle";
import { QuotaProvider } from "../../contexts/QuotaContext";

export default function DashboardLayout() {
  const { user, logout } = useAuthStore();

  return (
    <QuotaProvider>
      <div className="flex h-screen">
        <SidebarToggle />

        <div className="flex-1 flex flex-col">
          <header className="h-14 border-b flex items-center justify-between px-6">
            <span>Hello, {user?.email}</span>
            <button onClick={logout} className="text-sm underline">
              Logout
            </button>
          </header>
          <main className="flex-1 overflow-y-auto p-6">
            <Outlet />
          </main>
          {/* Global upload toaster */}
          <UploadList />
        </div>
      </div>
    </QuotaProvider>
  );
}
