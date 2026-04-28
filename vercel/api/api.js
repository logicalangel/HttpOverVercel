export const config = { runtime: "edge" };

import { Redis } from "@upstash/redis/cloudflare";

const AUTH_KEY = process.env.AUTH_KEY || "";
const UPSTREAM_TIMEOUT_MS = parseInt(process.env.UPSTREAM_TIMEOUT_MS || "30000", 10);

const SKIP_REQUEST_HEADERS = new Set([
  "host",
  "connection",
  "content-length",
  "transfer-encoding",
  "proxy-connection",
  "proxy-authorization",
]);

function log(level, msg, extra = {}) {
  console.log(JSON.stringify({ level, msg, ts: new Date().toISOString(), ...extra }));
}

// Record stats in background — failures are silently swallowed so the proxy
// keeps working even when Redis is not configured.
function getRedis() {
  try { return Redis.fromEnv(); } catch { return null; }
}

async function recordStats(domain, bytes) {
  const redis = getRedis();
  if (!redis) return;
  try {
    const today = new Date().toISOString().slice(0, 10);
    await Promise.all([
      redis.incr("stats:total"),
      redis.incrby("stats:bytes", bytes || 0),
      redis.zincrby("stats:domains", 1, domain || "unknown"),
      redis.incr(`stats:daily:${today}`),
    ]);
  } catch { /* Redis not configured or unavailable */ }
}

async function recordError() {
  const redis = getRedis();
  if (!redis) return;
  try { await redis.incr("stats:errors"); } catch {}
}

export default async function handler(request, context) {
  const start = Date.now();

  if (request.method !== "POST") {
    log("warn", "method not allowed", { method: request.method });
    return new Response("POST only", { status: 405 });
  }

  // Auth check
  const authKey = request.headers.get("x-auth-key");
  if (!AUTH_KEY || authKey !== AUTH_KEY) {
    log("warn", "unauthorized", { ip: request.headers.get("x-forwarded-for") });
    return new Response("Unauthorized", { status: 401 });
  }

  // Parse relay parameters from headers
  const method = (request.headers.get("x-relay-method") || "GET").toUpperCase();

  const urlB64 = request.headers.get("x-relay-url");
  if (!urlB64) {
    log("error", "missing x-relay-url");
    return new Response("missing x-relay-url", { status: 400 });
  }

  let targetURL;
  try {
    targetURL = atob(urlB64);
  } catch {
    log("error", "invalid x-relay-url base64");
    return new Response("invalid x-relay-url", { status: 400 });
  }

  if (!/^https?:\/\//i.test(targetURL)) {
    log("error", "bad url scheme", { url: targetURL });
    return new Response("bad url scheme", { status: 400 });
  }

  // Decode and forward request headers
  const headers = new Headers();
  const hdrsB64 = request.headers.get("x-relay-headers");
  if (hdrsB64) {
    try {
      const obj = JSON.parse(atob(hdrsB64));
      for (const [k, v] of Object.entries(obj)) {
        if (!SKIP_REQUEST_HEADERS.has(k.toLowerCase())) {
          headers.set(k, String(v));
        }
      }
    } catch {
      log("warn", "malformed x-relay-headers, ignoring");
    }
  }

  // Read request body (raw bytes) only for methods that carry a body
  const body =
    method !== "GET" && method !== "HEAD"
      ? await request.arrayBuffer()
      : undefined;

  log("info", "relay", { method, url: targetURL, bodyBytes: body?.byteLength ?? 0 });

  // Fetch upstream target
  let upResp;
  try {
    upResp = await fetch(targetURL, {
      method,
      headers,
      body: body && body.byteLength > 0 ? body : undefined,
      redirect: "follow",
      signal: AbortSignal.timeout(UPSTREAM_TIMEOUT_MS),
    });
  } catch (err) {
    const isTimeout = err?.name === "TimeoutError" || err?.name === "AbortError";
    const status = isTimeout ? 504 : 502;
    const label = isTimeout ? "upstream timeout" : "upstream fetch failed";
    log("error", label, { url: targetURL, err: String(err), ms: Date.now() - start });
    context.waitUntil(recordError());
    return new Response(`${label}: ${err}`, { status });
  }

  const ms = Date.now() - start;
  log("info", "done", { method, url: targetURL, status: upResp.status, ms });

  // Collect response headers as JSON → base64
  const respHdrs = {};
  for (const [k, v] of upResp.headers.entries()) {
    respHdrs[k] = v;
  }
  const respHdrsB64 = btoa(JSON.stringify(respHdrs));

  // Record stats in background (non-blocking)
  let domain = "unknown";
  try { domain = new URL(targetURL).hostname; } catch {}
  const contentLen = parseInt(upResp.headers.get("content-length") || "0", 10) || 0;
  context.waitUntil(recordStats(domain, contentLen));

  // Return raw upstream body; use transport status 200 so the Go client can
  // read x-relay-status to learn the actual upstream status code.
  return new Response(upResp.body, {
    status: 200,
    headers: {
      "x-relay-status": String(upResp.status),
      "x-relay-resp-headers": respHdrsB64,
      "content-type":
        upResp.headers.get("content-type") || "application/octet-stream",
    },
  });
}

  const start = Date.now();

  if (request.method !== "POST") {
    log("warn", "method not allowed", { method: request.method });
    return new Response("POST only", { status: 405 });
  }

  // Auth check
  const authKey = request.headers.get("x-auth-key");
  if (!AUTH_KEY || authKey !== AUTH_KEY) {
    log("warn", "unauthorized", { ip: request.headers.get("x-forwarded-for") });
    return new Response("Unauthorized", { status: 401 });
  }

  // Parse relay parameters from headers
  const method = (request.headers.get("x-relay-method") || "GET").toUpperCase();

  const urlB64 = request.headers.get("x-relay-url");
  if (!urlB64) {
    log("error", "missing x-relay-url");
    return new Response("missing x-relay-url", { status: 400 });
  }

  let targetURL;
  try {
    targetURL = atob(urlB64);
  } catch {
    log("error", "invalid x-relay-url base64");
    return new Response("invalid x-relay-url", { status: 400 });
  }

  if (!/^https?:\/\//i.test(targetURL)) {
    log("error", "bad url scheme", { url: targetURL });
    return new Response("bad url scheme", { status: 400 });
  }

  // Decode and forward request headers
  const headers = new Headers();
  const hdrsB64 = request.headers.get("x-relay-headers");
  if (hdrsB64) {
    try {
      const obj = JSON.parse(atob(hdrsB64));
      for (const [k, v] of Object.entries(obj)) {
        if (!SKIP_REQUEST_HEADERS.has(k.toLowerCase())) {
          headers.set(k, String(v));
        }
      }
    } catch {
      log("warn", "malformed x-relay-headers, ignoring");
    }
  }

  // Read request body (raw bytes) only for methods that carry a body
  const body =
    method !== "GET" && method !== "HEAD"
      ? await request.arrayBuffer()
      : undefined;

  log("info", "relay", { method, url: targetURL, bodyBytes: body?.byteLength ?? 0 });

  // Fetch upstream target
  let upResp;
  try {
    upResp = await fetch(targetURL, {
      method,
      headers,
      body: body && body.byteLength > 0 ? body : undefined,
      redirect: "follow",
      signal: AbortSignal.timeout(UPSTREAM_TIMEOUT_MS),
    });
  } catch (err) {
    const isTimeout = err?.name === "TimeoutError" || err?.name === "AbortError";
    const status = isTimeout ? 504 : 502;
    const label = isTimeout ? "upstream timeout" : "upstream fetch failed";
    log("error", label, { url: targetURL, err: String(err), ms: Date.now() - start });
    return new Response(`${label}: ${err}`, { status });
  }

  const ms = Date.now() - start;
  log("info", "done", { method, url: targetURL, status: upResp.status, ms });

  // Collect response headers as JSON → base64
  const respHdrs = {};
  for (const [k, v] of upResp.headers.entries()) {
    respHdrs[k] = v;
  }
  const respHdrsB64 = btoa(JSON.stringify(respHdrs));

  // Return raw upstream body; use transport status 200 so the Go client can
  // read x-relay-status to learn the actual upstream status code.
  return new Response(upResp.body, {
    status: 200,
    headers: {
      "x-relay-status": String(upResp.status),
      "x-relay-resp-headers": respHdrsB64,
      "content-type":
        upResp.headers.get("content-type") || "application/octet-stream",
    },
  });
}
