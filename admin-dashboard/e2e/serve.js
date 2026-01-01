/* eslint-disable no-console */

const http = require("node:http");
const path = require("node:path");
const fs = require("node:fs");
const { randomBytes } = require("node:crypto");

const port = Number.parseInt(process.env.E2E_FRONTEND_PORT ?? "39081", 10);
const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? "test-admin-password";

const state = {
  merchants: [],
  nextMerchantN: 1,
};

const outDir = path.resolve(__dirname, "../out");
const basePath = "/admin";

const types = new Map([
  [".html", "text/html; charset=utf-8"],
  [".js", "application/javascript; charset=utf-8"],
  [".css", "text/css; charset=utf-8"],
  [".ico", "image/x-icon"],
  [".svg", "image/svg+xml"],
  [".txt", "text/plain; charset=utf-8"],
  [".woff2", "font/woff2"],
  [".json", "application/json; charset=utf-8"],
]);

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

function safeJoin(root, rel) {
  const full = path.resolve(root, rel);
  const rootFull = path.resolve(root) + path.sep;
  if (!full.startsWith(rootFull)) return null;
  return full;
}

function sendText(res, status, body) {
  res.writeHead(status, { "Content-Type": "text/plain; charset=utf-8" });
  res.end(body);
}

function serveStatic(res, reqPath) {
  if (reqPath === basePath) {
    res.statusCode = 307;
    res.setHeader("Location", `${basePath}/`);
    res.end();
    return true;
  }
  if (!reqPath.startsWith(basePath + "/")) return false;

  let rel = reqPath.slice(basePath.length); // includes leading /
  try {
    rel = decodeURIComponent(rel);
  } catch {
    return false;
  }
  if (rel.endsWith("/")) rel += "index.html";
  rel = rel.replace(/^\/+/, "");

  const full = safeJoin(outDir, rel);
  if (!full) return false;

  let st;
  try {
    st = fs.statSync(full);
  } catch {
    const notFound = safeJoin(outDir, "404.html");
    if (notFound && fs.existsSync(notFound)) {
      res.writeHead(404, { "Content-Type": "text/html; charset=utf-8" });
      fs.createReadStream(notFound).pipe(res);
      return true;
    }
    sendText(res, 404, "not found\n");
    return true;
  }

  if (st.isDirectory()) {
    sendText(res, 404, "not found\n");
    return true;
  }

  const ext = path.extname(full);
  const ct = types.get(ext) ?? "application/octet-stream";
  res.writeHead(200, { "Content-Type": ct });
  fs.createReadStream(full).pipe(res);
  return true;
}

const server = http.createServer(async (req, res) => {
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

    if (req.method === "GET" || req.method === "HEAD") {
      if (serveStatic(res, url.pathname)) return;
    }

    sendJSON(res, 404, { status: "error", error: { code: "not_found", message: "not found" } });
  } catch (e) {
    sendJSON(res, 500, { status: "error", error: { code: "internal", message: e instanceof Error ? e.message : "internal error" } });
  }
});

server.listen(port, "127.0.0.1", () => {
  console.log(`[e2e] serving admin UI at http://127.0.0.1:${port}${basePath}/`);
});
