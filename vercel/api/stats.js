export const config = { runtime: "edge" };

import { Redis } from "@upstash/redis/cloudflare";

const STATS_USER = (process.env.STATS_USER || "").trim();
const STATS_PASS = (process.env.STATS_PASS || "").trim();

function checkAuth(authHeader) {
  if (!authHeader || !authHeader.startsWith("Basic ")) return false;
  let decoded;
  try { decoded = atob(authHeader.slice(6)); } catch { return false; }
  const colon = decoded.indexOf(":");
  if (colon === -1) return false;
  const user = decoded.slice(0, colon);
  const pass = decoded.slice(colon + 1);
  return STATS_USER && user === STATS_USER && pass === STATS_PASS;
}

export default async function handler(request) {
  if (!checkAuth(request.headers.get("authorization"))) {
    return new Response("Unauthorized", {
      status: 401,
      headers: { "WWW-Authenticate": 'Basic realm="HttpOverVercel Stats"' },
    });
  }

  let total = 0, bytes = 0, errors = 0, domains = [];
  let redisError = null;
  try {
    const redis = Redis.fromEnv();
    [total, bytes, errors, domains] = await Promise.all([
      redis.get("stats:total"),
      redis.get("stats:bytes"),
      redis.get("stats:errors"),
      redis.zrange("stats:domains", 0, 24, { rev: true, withScores: true }),
    ]);
    total  = Number(total)  || 0;
    bytes  = Number(bytes)  || 0;
    errors = Number(errors) || 0;
    domains = Array.isArray(domains) ? domains : [];
  } catch {
    redisError = true;
  }

  // @upstash/redis returns [{member,score},...] or interleaved [m,s,m,s,...]
  const domainRows = [];
  if (domains.length > 0 && typeof domains[0] === "object") {
    for (const item of domains) domainRows.push({ domain: item.member, count: item.score });
  } else {
    for (let i = 0; i + 1 < domains.length; i += 2)
      domainRows.push({ domain: domains[i], count: domains[i + 1] });
  }

  return new Response(renderHTML(total, bytes, errors, domainRows, redisError), {
    headers: { "content-type": "text/html; charset=utf-8" },
  });
}

function fmtBytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1048576) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1073741824) return `${(n / 1048576).toFixed(1)} MB`;
  return `${(n / 1073741824).toFixed(2)} GB`;
}

function esc(s) {
  return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function renderHTML(total, bytes, errors, domains, redisError) {
  const maxCount = domains[0]?.count || 1;

  const rows = domains.map((d, i) => {
    const pct = Math.max(2, Math.round((d.count / maxCount) * 180));
    return `<tr>
      <td class="num">${i + 1}</td>
      <td class="domain">${esc(d.domain)}</td>
      <td class="num">${Number(d.count).toLocaleString()}</td>
      <td><div class="bar" style="width:${pct}px"></div></td>
    </tr>`;
  }).join("\n");

  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>HttpOverVercel — Stats</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,-apple-system,sans-serif;background:#0f1117;color:#e2e8f0;padding:2rem;min-height:100vh}
h1{font-size:1.4rem;font-weight:700;margin-bottom:1.5rem;color:#f8fafc;display:flex;align-items:center;gap:.5rem}
h2{font-size:.7rem;font-weight:600;margin:2rem 0 .75rem;color:#64748b;text-transform:uppercase;letter-spacing:.08em}
.notice{background:#1c1a10;border:1px solid #854d0e;border-radius:8px;padding:.75rem 1rem;margin-bottom:1.5rem;font-size:.8rem;color:#fbbf24}
.cards{display:flex;gap:1rem;flex-wrap:wrap;margin-bottom:.5rem}
.card{background:#1e2532;border:1px solid #2d3748;border-radius:10px;padding:1.1rem 1.4rem;min-width:140px}
.card .label{font-size:.7rem;color:#64748b;text-transform:uppercase;letter-spacing:.06em;margin-bottom:.3rem}
.card .value{font-size:1.7rem;font-weight:700;color:#f8fafc;line-height:1}
.card.red .value{color:#f87171}
table{width:100%;border-collapse:collapse;background:#1e2532;border:1px solid #2d3748;border-radius:10px;overflow:hidden}
th{text-align:left;padding:.5rem .75rem;font-size:.68rem;color:#64748b;border-bottom:1px solid #2d3748;text-transform:uppercase;letter-spacing:.07em;background:#161c29}
td{padding:.45rem .75rem;border-bottom:1px solid #161c29;font-size:.82rem}
tr:last-child td{border-bottom:none}
tr:hover td{background:#232b3a}
td.num{color:#94a3b8;width:3rem;text-align:right}
td.domain{font-family:ui-monospace,monospace;color:#7dd3fc}
.bar{height:7px;background:#3b82f6;border-radius:3px}
.ts{margin-top:1.5rem;font-size:.7rem;color:#374151}
</style>
</head>
<body>
<h1>🌐 HttpOverVercel &mdash; Stats</h1>

${redisError ? `<div class="notice">⚠️ Redis not configured — stats are unavailable. Add Upstash Redis via <strong>Vercel → Storage</strong> to enable tracking.</div>` : ""}

<h2>Overview</h2>
<div class="cards">
  <div class="card">
    <div class="label">Total Requests</div>
    <div class="value">${total.toLocaleString()}</div>
  </div>
  <div class="card">
    <div class="label">Bytes Relayed</div>
    <div class="value">${fmtBytes(bytes)}</div>
  </div>
  <div class="card red">
    <div class="label">Errors</div>
    <div class="value">${errors.toLocaleString()}</div>
  </div>
</div>

<h2>Top Domains</h2>
${domains.length === 0
  ? `<p style="color:#475569;font-size:.875rem;padding:.5rem 0">${redisError ? "Unavailable — Redis not configured." : "No data yet — start using the proxy."}</p>`
  : `<table>
<thead><tr><th>#</th><th>Domain</th><th style="text-align:right">Requests</th><th></th></tr></thead>
<tbody>
${rows}
</tbody>
</table>`}

<p class="ts">Last refreshed: ${new Date().toUTCString()} &bull; <a href="" style="color:#475569">Refresh</a></p>
</body>
</html>`;
}
