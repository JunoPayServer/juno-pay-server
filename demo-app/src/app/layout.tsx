import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Juno Pay Demo",
  description: "Demo checkout UI for juno-pay-server",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className="bg-zinc-50">
      <body className="min-h-screen bg-zinc-50 text-zinc-950">{children}</body>
    </html>
  );
}
