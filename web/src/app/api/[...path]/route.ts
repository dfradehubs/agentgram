export const dynamic = "force-dynamic";
export const revalidate = 0;
export const runtime = "nodejs";

import logger from "@/lib/logger";

const getBackendUrl = () =>
  process.env.BACKEND_URL || "http://localhost:8080";

const FORWARDED_HEADERS = [
  "content-type",
  "authorization",
  "accept",
  "cookie",
];

async function proxy(
  request: Request,
  { params }: { params: Promise<{ path: string[] }> }
): Promise<Response> {
  const start = Date.now();
  const { path } = await params;
  const targetPath = `/api/${path.join("/")}`;

  logger.info({ method: request.method, path: targetPath }, "proxy request");

  const url = new URL(targetPath, getBackendUrl());

  const { searchParams } = new URL(request.url);
  searchParams.forEach((value, key) => url.searchParams.set(key, value));

  const headers = new Headers();
  for (const name of FORWARDED_HEADERS) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }

  const init: RequestInit = {
    method: request.method,
    headers,
    cache: "no-store",
  };

  if (request.method !== "GET" && request.method !== "HEAD") {
    init.body = await request.text();
  }

  let upstream: Response;
  try {
    upstream = await fetch(url.toString(), init);
  } catch (err) {
    const duration = Date.now() - start;
    logger.error({ method: request.method, path: targetPath, duration, err }, "upstream error");
    return new Response(JSON.stringify({ error: "upstream unavailable" }), {
      status: 502,
      headers: { "Content-Type": "application/json" },
    });
  }

  const ct = upstream.headers.get("content-type") || "";
  const duration = Date.now() - start;

  logger.info(
    { method: request.method, path: targetPath, status: upstream.status, duration, contentType: ct },
    "proxy response"
  );

  if (ct.includes("text/event-stream")) {
    return new Response(upstream.body, {
      status: upstream.status,
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache, no-transform",
        Connection: "keep-alive",
        "X-Accel-Buffering": "no",
      },
    });
  }

  return new Response(upstream.body, {
    status: upstream.status,
    headers: { "Content-Type": ct },
  });
}

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
