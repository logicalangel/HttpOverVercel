export const runtime = "edge";

const AUTH_KEY = process.env.AUTH_KEY || "";

const SKIP_REQUEST_HEADERS = new Set([
  "host",
  "connection",
  "content-length",
  "transfer-encoding",
  "proxy-connection",
  "proxy-authorization",
]);

export default async function handler(request) {
  if (request.method !== "POST") {
    return new Response("POST only", { status: 405 });
  }

  // Auth check
  const authKey = request.headers.get("x-auth-key");
  if (!AUTH_KEY || authKey !== AUTH_KEY) {
    return new Response("Unauthorized", { status: 401 });
  }

  // Parse relay parameters from headers
  const method = (request.headers.get("x-relay-method") || "GET").toUpperCase();

  const urlB64 = request.headers.get("x-relay-url");
  if (!urlB64) {
    return new Response("missing x-relay-url", { status: 400 });
  }

  let targetURL;
  try {
    targetURL = atob(urlB64);
  } catch {
    return new Response("invalid x-relay-url", { status: 400 });
  }

  if (!/^https?:\/\//i.test(targetURL)) {
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
      // ignore malformed headers
    }
  }

  // Read request body (raw bytes) only for methods that carry a body
  const body =
    method !== "GET" && method !== "HEAD"
      ? await request.arrayBuffer()
      : undefined;

  // Fetch upstream target
  let upResp;
  try {
    upResp = await fetch(targetURL, {
      method,
      headers,
      body: body && body.byteLength > 0 ? body : undefined,
      redirect: "follow",
    });
  } catch (err) {
    return new Response(`upstream fetch failed: ${err}`, { status: 502 });
  }

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
