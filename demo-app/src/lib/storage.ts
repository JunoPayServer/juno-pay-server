export type DemoUser = {
  user_id: string;
  email: string;
  username?: string;
};

export type DemoOrder = {
  order_id: string;
  external_order_id: string;
  invoice_id: string;
  invoice_token: string;
  address: string;
  amount_zat: number;
  status: string;
  received_zat_pending: number;
  received_zat_confirmed: number;
  created_at: string;
  updated_at: string;
  events_cursor: string;
};

const USER_KEY = "juno_demo_user_v1";
const ORDERS_KEY = "juno_demo_orders_v1";
export const DEMO_USER: DemoUser = {
  user_id: "demo-user",
  email: "demo@junopayserver.com",
  username: "Demo",
};

export function loadUser(): DemoUser | null {
  if (typeof window === "undefined") return null;
  const raw = window.localStorage.getItem(USER_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as DemoUser;
  } catch {
    return null;
  }
}

export function saveUser(u: DemoUser) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(USER_KEY, JSON.stringify(u));
}

export function loadOrCreateDemoUser(): DemoUser {
  const existing = loadUser();
  if (existing) return existing;
  saveUser(DEMO_USER);
  return DEMO_USER;
}

export function clearUser() {
  if (typeof window === "undefined") return;
  window.localStorage.removeItem(USER_KEY);
}

export function loadOrders(): DemoOrder[] {
  if (typeof window === "undefined") return [];
  const raw = window.localStorage.getItem(ORDERS_KEY);
  if (!raw) return [];
  try {
    const v = JSON.parse(raw) as DemoOrder[];
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
}

export function saveOrders(orders: DemoOrder[]) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(ORDERS_KEY, JSON.stringify(orders));
}
