import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Juno Pay Demo",
  description: "Demo checkout UI for juno-pay-server",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* Apply saved theme before React hydrates to prevent flash */}
        <script
          dangerouslySetInnerHTML={{
            __html: `(function(){var m=document.cookie.match(/(?:^|;\\s*)juno_theme_v1=([^;]+)/);var t=(m&&m[1])||localStorage.getItem('juno_theme_v1')||'dark';if(!m){document.cookie='juno_theme_v1='+t+';path=/;max-age=31536000;SameSite=Lax';}document.documentElement.setAttribute('data-theme',t);})();`,
          }}
        />
      </head>
      <body className="min-h-screen th-page th-text">{children}</body>
    </html>
  );
}
