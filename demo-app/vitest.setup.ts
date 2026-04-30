import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

if (typeof window !== "undefined" && typeof window.localStorage?.clear !== "function") {
  const data = new Map<string, string>();
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: {
      getItem(key: string) {
        return data.has(key) ? data.get(key)! : null;
      },
      setItem(key: string, value: string) {
        data.set(key, String(value));
      },
      removeItem(key: string) {
        data.delete(key);
      },
      clear() {
        data.clear();
      },
      key(index: number) {
        return Array.from(data.keys())[index] ?? null;
      },
      get length() {
        return data.size;
      },
    },
  });
}

afterEach(() => {
  cleanup();
});
