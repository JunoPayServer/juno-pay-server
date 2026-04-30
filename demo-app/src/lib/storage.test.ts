import { beforeEach, describe, expect, it } from "vitest";
import { DEMO_USER, clearUser, loadOrCreateDemoUser, loadOrders, loadUser, saveOrders, saveUser, type DemoOrder } from "@/lib/storage";

describe("storage", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("returns null user when missing", () => {
    expect(loadUser()).toBeNull();
  });

  it("round-trips user", () => {
    saveUser({ user_id: "u1", email: "a@b.com" });
    expect(loadUser()).toEqual({ user_id: "u1", email: "a@b.com" });
  });

  it("creates the default demo user when missing", () => {
    expect(loadOrCreateDemoUser()).toEqual(DEMO_USER);
    expect(loadUser()).toEqual(DEMO_USER);
  });

  it("clearUser removes user", () => {
    saveUser({ user_id: "u1", email: "a@b.com" });
    clearUser();
    expect(loadUser()).toBeNull();
  });

  it("returns empty orders when missing", () => {
    expect(loadOrders()).toEqual([]);
  });

  it("round-trips orders", () => {
    const o: DemoOrder = {
      order_id: "o1",
      external_order_id: "ext1",
      invoice_id: "inv_1",
      invoice_token: "tok",
      address: "j1...",
      amount_zat: 100,
      status: "open",
      received_zat_pending: 0,
      received_zat_confirmed: 0,
      created_at: "2025-01-01T00:00:00Z",
      updated_at: "2025-01-01T00:00:00Z",
      events_cursor: "0",
    };
    saveOrders([o]);
    expect(loadOrders()).toEqual([o]);
  });

  it("returns empty orders for invalid JSON", () => {
    window.localStorage.setItem("juno_demo_orders_v1", "{");
    expect(loadOrders()).toEqual([]);
  });

  it("returns empty orders when value is not an array", () => {
    window.localStorage.setItem("juno_demo_orders_v1", JSON.stringify({ nope: true }));
    expect(loadOrders()).toEqual([]);
  });
});
