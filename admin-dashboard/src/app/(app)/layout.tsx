import { AdminProvider } from "@/components/AdminProvider";
import { Sidebar } from "@/components/Sidebar";
import { TopBar } from "@/components/TopBar";

export default function AppLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <AdminProvider>
      <div className="flex h-full min-h-screen bg-zinc-50 text-zinc-950">
        <Sidebar />
        <div className="flex min-w-0 flex-1 flex-col">
          <TopBar />
          <main className="flex-1 p-6">{children}</main>
        </div>
      </div>
    </AdminProvider>
  );
}

