/* eslint-disable no-console */

const http = require("node:http");
const { spawn } = require("node:child_process");
const { randomBytes } = require("node:crypto");
const path = require("node:path");

const backendPort = Number.parseInt(process.env.E2E_BACKEND_PORT ?? "39080", 10);
const frontendPort = Number.parseInt(process.env.E2E_FRONTEND_PORT ?? "39081", 10);
const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? "test-admin-password";

const state = {
  merchants: [],
  nextMerchantN: 1,
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

function parseCookies(req) {
  const header = req.headers.cookie ?? "";
  const out = {};
  for (const part of header.split(";")) {
    const p = part.trim();
    if (!p) continue;
    const idx = p.indexOf("=");
    if (idx === -1) continue;
    out[p.slice(0, idx)] = p.slice(idx + 1);
  }
  return out;
}

function isAuthed(req) {
  const cookies = parseCookies(req);
  return Boolean(cookies.juno_admin_session);
}

function requireAuth(req, res) {
  if (isAuthed(req)) return true;
  sendJSON(res, 401, { status: "error", error: { code: "unauthorized", message: "unauthorized" } });
  return false;
}

const backend = http.createServer(async (req, res) => {
  try {
    const url = new URL(req.url ?? "/", `http://${req.headers.host ?? "127.0.0.1"}`);

    if (req.method === "POST" && url.pathname === "/admin/login") {
      const raw = await readBody(req);
      let body;
      try {
        body = JSON.parse(raw || "{}");
      } catch {
        body = {};
      }
      if (body.password !== adminPassword) {
        return sendJSON(res, 401, { status: "error", error: { code: "unauthorized", message: "wrong password" } });
      }

      const exp = Math.floor(Date.now() / 1000) + 3600;
      const payload = `v1.${exp}.${randomBytes(8).toString("hex")}.sig`;
      res.setHeader("Set-Cookie", `juno_admin_session=${payload}; Path=/; HttpOnly; SameSite=Lax`);
      res.writeHead(204);
      return res.end();
    }

    if (req.method === "POST" && url.pathname === "/admin/logout") {
      res.setHeader("Set-Cookie", "juno_admin_session=; Path=/; HttpOnly; Max-Age=-1; SameSite=Lax");
      res.writeHead(204);
      return res.end();
    }

    if (req.method === "GET" && url.pathname === "/v1/admin/status") {
      if (!requireAuth(req, res)) return;
      return sendJSON(res, 200, {
        status: "ok",
        data: {
          chain: { best_height: 42, best_hash: "00", uptime_seconds: 123 },
          scanner: { connected: true, last_cursor_applied: 7, last_event_at: null },
          event_delivery: { pending_deliveries: 0 },
          restarts: { junocashd_restarts_detected: 0, last_restart_at: null },
        },
      });
    }

    if (req.method === "GET" && url.pathname === "/v1/admin/merchants") {
      if (!requireAuth(req, res)) return;
      return sendJSON(res, 200, { status: "ok", data: state.merchants });
    }

    if (req.method === "POST" && url.pathname === "/v1/admin/merchants") {
      if (!requireAuth(req, res)) return;
      const raw = await readBody(req);
      let body;
      try {
        body = JSON.parse(raw || "{}");
      } catch {
        body = {};
      }
      const name = typeof body.name === "string" ? body.name.trim() : "";
      if (!name) {
        return sendJSON(res, 400, { status: "error", error: { code: "bad_request", message: "name is required" } });
      }
      const ts = nowRFC3339();
      const m = {
        merchant_id: `m_${state.nextMerchantN++}`,
        name,
        status: "active",
        settings: body.settings ?? {
          invoice_ttl_seconds: 3600,
          required_confirmations: 100,
          policies: { late_payment_policy: "reject", partial_payment_policy: "reject", overpayment_policy: "accept" },
        },
        created_at: ts,
        updated_at: ts,
      };
      state.merchants.unshift(m);
      return sendJSON(res, 201, { status: "ok", data: m });
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
    NEXT_TELEMETRY_DISABLED: "1",
  },
});

function shutdown() {
  backend.close();
  nextProc.kill("SIGTERM");
}

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);

nextProc.on("exit", (code) => {
  backend.close();
  process.exit(code ?? 0);
});

