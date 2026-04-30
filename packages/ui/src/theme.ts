export type Theme = "dark" | "light";

const KEY = "juno_theme_v1";

function readCookie(): Theme | null {
  if (typeof document === "undefined") return null;
  const m = document.cookie.match(/(?:^|;\s*)juno_theme_v1=([^;]+)/);
  return m ? (m[1] as Theme) : null;
}

function writeCookie(t: Theme) {
  document.cookie = `${KEY}=${t};path=/;max-age=31536000;SameSite=Lax`;
}

export function getTheme(): Theme {
  if (typeof window === "undefined") return "dark";
  const cookie = readCookie();
  if (cookie) return cookie;
  // No cookie yet — promote localStorage value to cookie so the other app
  // (different port, same hostname) can read it on next load.
  const local = (window.localStorage.getItem(KEY) as Theme) ?? "dark";
  writeCookie(local);
  return local;
}

export function setTheme(t: Theme) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(KEY, t);
  writeCookie(t);
  document.documentElement.setAttribute("data-theme", t);
}
