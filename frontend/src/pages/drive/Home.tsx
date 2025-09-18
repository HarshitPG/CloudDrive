import { useAuthStore } from "../../stores/auth";

export default function HomePage() {
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);

  return (
    <div className="p-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl">FileVault</h1>
        <div>
          <span className="mr-4">{user?.email}</span>
          <button className="px-3 py-1 border rounded" onClick={() => logout()}>
            Logout
          </button>
        </div>
      </div>
      <div className="mt-8">
        <p>Welcome to Cloud Drive.</p>
      </div>
    </div>
  );
}
