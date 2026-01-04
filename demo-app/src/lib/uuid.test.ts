import { afterEach, describe, expect, it, vi } from "vitest";
import { uuidv4 } from "@/lib/uuid";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("uuidv4", () => {
  it("generates v4 UUIDs via getRandomValues", () => {
    vi.stubGlobal(
      "crypto",
      {
        getRandomValues(arr: Uint8Array) {
          for (let i = 0; i < arr.length; i++) {
            arr[i] = i;
          }
          return arr;
        },
      } as unknown as Crypto,
    );

    expect(uuidv4()).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  });

  it("falls back when crypto is unavailable", () => {
    vi.stubGlobal("crypto", undefined as unknown as Crypto);
    expect(uuidv4()).toMatch(/^demo-[0-9a-f]+-[0-9a-f]+$/);
  });
});

