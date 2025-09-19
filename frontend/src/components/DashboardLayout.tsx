import { type ReactNode } from "react";
import { useAuthStore } from "../stores/auth";
import QuotaWidget from "./QuotaWidget";

export default function DashboardLayout({ children }: { children: ReactNode }) {
  const { user, logout } = useAuthStore();

  return (
    <div className="flex h-screen">
      {/* Sidebar */}
      <aside className="w-64 bg-gray-100 p-4 flex flex-col">
        <h2 className="font-bold text-xl mb-6">FileVault</h2>
        <nav className="flex-1 space-y-3">
          <button className="w-full text-left">My Drive</button>
          <button className="w-full text-left">Starred</button>
          <button className="w-full text-left">Trash</button>
        </nav>
        <QuotaWidget />
      </aside>

      {/* Main */}
      <div className="flex-1 flex flex-col">
        <header className="h-14 border-b flex items-center justify-between px-6">
          <span>Hello, {user?.email}</span>
          <button onClick={logout} className="text-sm underline">
            Logout
          </button>
        </header>
        <main className="flex-1 overflow-y-auto p-6">{children}</main>
      </div>
    </div>
  );
}
