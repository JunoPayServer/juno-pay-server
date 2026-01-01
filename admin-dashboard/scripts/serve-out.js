/* eslint-disable no-console */

const http = require("node:http");
const fs = require("node:fs");
const path = require("node:path");

const port = Number.parseInt(process.env.PORT ?? "3000", 10);
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

function send(res, status, body, headers) {
  res.writeHead(status, headers ?? {});
  res.end(body);
}

function safeJoin(root, rel) {
  const full = path.resolve(root, rel);
  const rootFull = path.resolve(root) + path.sep;
  if (!full.startsWith(rootFull)) return null;
  return full;
}

function fileFor(reqPath) {
  if (reqPath === basePath) {
    return { redirect: `${basePath}/` };
  }
  if (!reqPath.startsWith(basePath + "/")) return null;

  let rel = reqPath.slice(basePath.length); // includes leading "/"
  rel = decodeURIComponent(rel);
  if (rel.endsWith("/")) rel += "index.html";
  rel = rel.replace(/^\/+/, "");

  const full = safeJoin(outDir, rel);
  if (!full) return null;
  return { full };
}

const srv = http.createServer((req, res) => {
  try {
    const url = new URL(req.url ?? "/", `http://${req.headers.host ?? "127.0.0.1"}`);
    const mapped = fileFor(url.pathname);
    if (!mapped) {
      return send(res, 404, "not found\n", { "Content-Type": "text/plain; charset=utf-8" });
    }
    if (mapped.redirect) {
      res.statusCode = 307;
      res.setHeader("Location", mapped.redirect);
      return res.end();
    }
    const st = fs.statSync(mapped.full);
    if (st.isDirectory()) {
      return send(res, 404, "not found\n", { "Content-Type": "text/plain; charset=utf-8" });
    }
    const ext = path.extname(mapped.full);
    const ct = types.get(ext) ?? "application/octet-stream";
    res.writeHead(200, { "Content-Type": ct });
    fs.createReadStream(mapped.full).pipe(res);
  } catch (e) {
    return send(res, 500, "internal error\n", { "Content-Type": "text/plain; charset=utf-8" });
  }
});

srv.listen(port, "127.0.0.1", () => {
  console.log(`[admin-ui] serving ${outDir} at http://127.0.0.1:${port}${basePath}/`);
});

