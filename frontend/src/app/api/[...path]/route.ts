import type { NextRequest } from "next/server";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

type RouteContext = { params: Promise<{ path: string[] }> };

const MAX_BODY_BYTES = 32_768;
const STATIC_GET_ROUTES = new Set([
  "/api/v1/health",
  "/api/v1/chains",
  "/api/v1/assets",
  "/api/v1/markets/overview",
  "/api/v1/intelligence/near",
  "/api/v1/research/scenarios",
  "/api/v1/activity",
  "/api/v1/risk/status"
]);

function isAllowedRoute(method: string, pathname: string): boolean {
  if (method === "GET") {
    return STATIC_GET_ROUTES.has(pathname)
      || /^\/api\/v1\/swaps\/[a-zA-Z0-9_-]+$/.test(pathname)
      || /^\/api\/v1\/trading\/(overview|signals|top-movers|history|backtests)$/.test(pathname)
      || /^\/api\/v1\/trading\/(assets|orderblocks|liquidity|scenarios)\/[a-zA-Z0-9_.:%-]+$/.test(pathname);
  }
  if (method === "POST") {
    return pathname === "/api/v1/quotes"
      || pathname === "/api/v1/swaps"
      || pathname === "/api/v1/auth/nonce"
      || pathname === "/api/v1/auth/verify"
      || pathname === "/api/v1/auth/logout"
      || /^\/api\/v1\/swaps\/[a-zA-Z0-9_-]+\/deposit$/.test(pathname)
      || /^\/api\/v1\/swaps\/[a-zA-Z0-9_-]+\/cancel$/.test(pathname)
      || /^\/api\/v1\/swaps\/[a-zA-Z0-9_-]+\/simulate$/.test(pathname);
  }
  return false;
}

async function readLimitedBody(request: Request): Promise<string | null> {
  const contentLength = Number(request.headers.get("content-length") ?? 0);
  if (contentLength > MAX_BODY_BYTES) return null;
  if (!request.body) return "";

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let totalBytes = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    totalBytes += value.byteLength;
    if (totalBytes > MAX_BODY_BYTES) {
      await reader.cancel();
      return null;
    }
    chunks.push(value);
  }
  const body = new Uint8Array(totalBytes);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return new TextDecoder().decode(body);
}

async function proxy(request: NextRequest, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  const pathname = `/api/${path.map((segment) => encodeURIComponent(segment)).join("/")}`;
  if (!isAllowedRoute(request.method, pathname)) {
    return Response.json({ error: { code: "NOT_FOUND", message: "API route was not found." } }, { status: 404 });
  }

  const incomingUrl = new URL(request.url);
  const upstreamUrl = new URL(pathname, process.env.UDI_API_ORIGIN ?? "http://127.0.0.1:8081");
  for (const key of ["chain", "refresh", "timeframe", "asset", "venue", "quote", "liquidityTier", "direction", "outcome", "from", "to", "page", "pageSize"]) {
    const value = incomingUrl.searchParams.get(key);
    if (value !== null) upstreamUrl.searchParams.set(key, value);
  }

  const headers = new Headers({ accept: "application/json" });
  const contentType = request.headers.get("content-type");
  if (contentType?.toLowerCase().startsWith("application/json")) {
    headers.set("content-type", "application/json");
  }
  const idempotencyKey = request.headers.get("idempotency-key");
  if (idempotencyKey && idempotencyKey.length <= 128) {
    headers.set("idempotency-key", idempotencyKey);
  }
  const authHeader = request.headers.get("authorization");
  if (authHeader) {
    headers.set("authorization", authHeader);
  }
  const cookieHeader = request.headers.get("cookie");
  if (cookieHeader) {
    headers.set("cookie", cookieHeader);
  }

  let body: string | undefined;
  if (request.method !== "GET" && request.method !== "HEAD") {
    if (contentType && !contentType.toLowerCase().startsWith("application/json")) {
      return Response.json({
        error: { code: "UNSUPPORTED_CONTENT_TYPE", message: "Requests must use application/json." }
      }, { status: 415 });
    }
    body = await readLimitedBody(request) ?? undefined;
    if (body === undefined) {
      return Response.json({
        error: { code: "BODY_TOO_LARGE", message: "Request body exceeds 32 KB." }
      }, { status: 413 });
    }
  }

  try {
    const upstream = await fetch(upstreamUrl, {
      method: request.method,
      headers,
      ...(body === undefined ? {} : { body }),
      cache: "no-store",
      redirect: "error",
      signal: AbortSignal.timeout(12_000)
    });
    const responseHeaders = new Headers({
      "cache-control": "no-store",
      "content-type": upstream.headers.get("content-type") ?? "application/json; charset=utf-8"
    });
    const requestId = upstream.headers.get("x-request-id");
    if (requestId) responseHeaders.set("x-request-id", requestId);
    return new Response(upstream.body, { status: upstream.status, headers: responseHeaders });
  } catch {
    return Response.json({
      error: {
        code: "API_UNAVAILABLE",
        message: "The local API is unavailable. Start the application with npm run dev."
      }
    }, { status: 502, headers: { "cache-control": "no-store" } });
  }
}

export const GET = proxy;
export const POST = proxy;
