import type { Metadata } from 'next'
import { Geist, Geist_Mono } from 'next/font/google'
import './globals.css'

const geistSans = Geist({
  variable: '--font-geist-sans',
  subsets: ['latin'],
})

const geistMono = Geist_Mono({
  variable: '--font-geist-mono',
  subsets: ['latin'],
})

export const metadata: Metadata = {
  title: 'Juno Pay Admin',
  description: 'Admin dashboard for Juno Pay Server',
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html
      lang="en"
      className="h-full"
      suppressHydrationWarning
    >
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `try{var m=document.cookie.match(/(?:^|;\\s*)juno_theme_v1=([^;]+)/);var t=(m&&m[1])||localStorage.getItem('juno_theme_v1')||'light';if(!m){document.cookie='juno_theme_v1='+t+';path=/;max-age=31536000;SameSite=Lax';}document.documentElement.setAttribute('data-theme',t);}catch(_){}`,
          }}
        />
      </head>
      <body className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}>{children}</body>
    </html>
  )
}
