/* eslint-disable no-console */

const http = require("node:http");
const { spawn } = require("node:child_process");
const crypto = require("node:crypto");
const path = require("node:path");

const backendPort = Number.parseInt(process.env.E2E_BACKEND_PORT ?? "39180", 10);
const frontendPort = Number.parseInt(process.env.E2E_FRONTEND_PORT ?? "39181", 10);
const merchantKey = process.env.JUNO_PAY_MERCHANT_API_KEY ?? "test-merchant-key";

const state = {
  invoicesByExternalOrderID: new Map(),
  invoices: new Map(),
  events: new Map(),
  nextInvoiceN: 1,
  nextEventN: 1,
  bestHeight: 100,
  confirmTimers: new Map(),
};

function nowRFC3339() {
  return new Date().toISOString();
}

function sendJSON(res, status, obj) {
  const body = JSON.stringify(obj);
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(body);
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    let data = "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => {
      data += chunk;
    });
    req.on("end", () => resolve(data));
    req.on("error", reject);
  });
}

function bearer(req) {
  const h = (req.headers.authorization ?? "").trim();
  const parts = h.split(" ");
  if (parts.length === 2 && parts[0].toLowerCase() === "bearer") return parts[1];
  return "";
}

function getInvoiceToken(inv) {
  const token = inv.invoice_token;
  return typeof token === "string" ? token : "";
}

function addEvent(invoiceId, type, deposit) {
  const id = String(state.nextEventN++);
  const e = {
    event_id: id,
    type,
    occurred_at: nowRFC3339(),
    invoice_id: invoiceId,
    deposit: deposit ?? null,
    refund: null,
  };
  const arr = state.events.get(invoiceId) ?? [];
  arr.push(e);
  state.events.set(invoiceId, arr);
  return e;
}

function listEventsAfter(invoiceId, afterID) {
  const arr = state.events.get(invoiceId) ?? [];
  const out = [];
  let nextCursor = 0;
  for (const e of arr) {
    const n = Number.parseInt(e.event_id, 10);
    if (!Number.isFinite(n) || n <= afterID) continue;
    out.push(e);
    nextCursor = n;
  }
  return { out, nextCursor };
}

const backend = http.createServer(async (req, res) => {
  try {
    const url = new URL(req.url ?? "/", `http://${req.headers.host ?? "127.0.0.1"}`);

    if (req.method === "GET" && url.pathname === "/v1/status") {
      return sendJSON(res, 200, {
        status: "ok",
        data: {
          chain: {
            best_height: state.bestHeight,
            best_hash: "e2e",
            uptime_seconds: 1,
          },
          scanner: {
            connected: true,
            last_cursor_applied: 0,
            last_event_at: null,
          },
          event_delivery: {
            pending_deliveries: 0,
          },
        },
      });
    }

    if (req.method === "POST" && url.pathname === "/v1/invoices") {
      if (bearer(req) !== merchantKey) {
        return sendJSON(res, 401, { status: "error", error: { code: "unauthorized", message: "unauthorized" } });
      }
      const raw = await readBody(req);
      let body;
      try {
        body = JSON.parse(raw || "{}");
      } catch {
        body = {};
      }
      const externalOrderID = typeof body.external_order_id === "string" ? body.external_order_id.trim() : "";
      const amountZat = typeof body.amount_zat === "number" ? body.amount_zat : 0;
      if (!externalOrderID) {
        return sendJSON(res, 400, { status: "error", error: { code: "invalid_argument", message: "external_order_id is required" } });
      }
      if (!Number.isFinite(amountZat) || amountZat <= 0) {
        return sendJSON(res, 400, { status: "error", error: { code: "invalid_argument", message: "amount_zat is required" } });
      }

      const existing = state.invoicesByExternalOrderID.get(externalOrderID);
      if (existing) {
        return sendJSON(res, 200, { status: "ok", data: existing });
      }

      const now = nowRFC3339();
      const invoiceId = `inv_${state.nextInvoiceN++}`;
      const token = crypto.randomBytes(16).toString("hex");
      const expiresAt = new Date(Date.now() + 15 * 60 * 1000).toISOString();
      const invoice = {
        invoice_id: invoiceId,
        merchant_id: "m_demo",
        external_order_id: externalOrderID,
        status: "open",
        address: `j1demo${crypto.randomBytes(16).toString("hex")}`,
        amount_zat: amountZat,
        required_confirmations: 100,
        received_zat_pending: 0,
        received_zat_confirmed: 0,
        expires_at: expiresAt,
        created_at: now,
        updated_at: now,
        policies: {
          late_payment_policy: "ignore",
          partial_payment_policy: "reject_partial",
          overpayment_policy: "mark_overpaid",
        },
      };

      const pub = { invoice, invoice_token: token };
      state.invoices.set(invoiceId, pub);
      state.invoicesByExternalOrderID.set(externalOrderID, pub);
      state.events.set(invoiceId, []);
      addEvent(invoiceId, "invoice.created");

      return sendJSON(res, 201, { status: "ok", data: pub });
    }

    if (req.method === "GET" && url.pathname.startsWith("/v1/public/invoices/")) {
      const parts = url.pathname.split("/").filter(Boolean);
      const invoiceId = parts[3] ?? "";
      const pub = state.invoices.get(invoiceId);
      if (!pub) {
        return sendJSON(res, 404, { status: "error", error: { code: "not_found", message: "invoice not found" } });
      }
      const token = url.searchParams.get("token") ?? "";
      if (token !== getInvoiceToken(pub)) {
        return sendJSON(res, 401, { status: "error", error: { code: "unauthorized", message: "invalid token" } });
      }

      if (parts.length === 4) {
        return sendJSON(res, 200, { status: "ok", data: pub.invoice });
      }

      if (parts.length === 5 && parts[4] === "events") {
        const afterID = Number.parseInt(url.searchParams.get("cursor") ?? "0", 10) || 0;
        const { out, nextCursor } = listEventsAfter(invoiceId, afterID);
        return sendJSON(res, 200, { status: "ok", data: { events: out, next_cursor: String(nextCursor) } });
      }
    }

    if (req.method === "POST" && url.pathname.startsWith("/_test/pay/")) {
      const invoiceId = url.pathname.split("/").pop() ?? "";
      const pub = state.invoices.get(invoiceId);
      if (!pub) {
        return sendJSON(res, 404, { status: "error", error: { code: "not_found", message: "invoice not found" } });
      }
      const now = nowRFC3339();
      pub.invoice.received_zat_pending = pub.invoice.amount_zat;
      pub.invoice.received_zat_confirmed = 0;
      pub.invoice.status = "pending";
      pub.invoice.updated_at = now;

      const deposit = {
        wallet_id: "w_demo",
        txid: crypto.randomBytes(32).toString("hex"),
        action_index: 0,
        amount_zat: pub.invoice.amount_zat,
        height: state.bestHeight,
      };
      addEvent(invoiceId, "deposit.detected", deposit);
      addEvent(invoiceId, "invoice.paid");

      if (!state.confirmTimers.has(invoiceId)) {
        const required = pub.invoice.required_confirmations || 0;
        const depositHeight = deposit.height;
        const timer = setInterval(() => {
          state.bestHeight += 1;
          const confs = state.bestHeight - depositHeight + 1;
          if (confs >= required) {
            clearInterval(timer);
            state.confirmTimers.delete(invoiceId);

            const now2 = nowRFC3339();
            pub.invoice.received_zat_confirmed = pub.invoice.amount_zat;
            pub.invoice.status = "confirmed";
            pub.invoice.updated_at = now2;
            addEvent(invoiceId, "deposit.confirmed", deposit);
          }
        }, 50);
        state.confirmTimers.set(invoiceId, timer);
      }

      return sendJSON(res, 200, { status: "ok", data: { paid: true } });
    }

    sendJSON(res, 404, { status: "error", error: { code: "not_found", message: "not found" } });
  } catch (e) {
    sendJSON(res, 500, { status: "error", error: { code: "internal", message: e instanceof Error ? e.message : "internal error" } });
  }
});

backend.listen(backendPort, "127.0.0.1", () => {
  console.log(`[e2e] mock backend on http://127.0.0.1:${backendPort}`);
});

const nextBin = path.resolve(__dirname, "../node_modules/.bin/next");
const nextProc = spawn(nextBin, ["start", "-p", String(frontendPort)], {
  stdio: "inherit",
  env: {
    ...process.env,
    JUNO_PAY_BASE_URL: `http://127.0.0.1:${backendPort}`,
    JUNO_PAY_MERCHANT_API_KEY: merchantKey,
    NEXT_TELEMETRY_DISABLED: "1",
  },
});

function shutdown() {
  for (const t of state.confirmTimers.values()) clearInterval(t);
  state.confirmTimers.clear();
  backend.close();
  nextProc.kill("SIGTERM");
}

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);

nextProc.on("exit", (code) => {
  backend.close();
  process.exit(code ?? 0);
});
