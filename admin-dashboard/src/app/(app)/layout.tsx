import { AdminProvider } from "@/components/AdminProvider";
import { Sidebar } from "@/components/Sidebar";
import { TopBar } from "@/components/TopBar";

export default function AppLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <AdminProvider>
      <div className="th-page min-h-screen">
        <Sidebar />
        <div className="ml-64 flex min-w-0 flex-1 flex-col min-h-screen th-content">
          <TopBar />
          <main className="flex-1 p-8">{children}</main>
        </div>
      </div>
    </AdminProvider>
  );
}
