const getBackendUrl = () =>
  process.env.BACKEND_URL || "http://localhost:8080";

async function proxy(
  request: Request,
  { params }: { params: Promise<{ path: string[] }> }
): Promise<Response> {
  const { path } = await params;
  const targetPath = `/auth/${path.join("/")}`;

  const url = new URL(targetPath, getBackendUrl());

  const { searchParams } = new URL(request.url);
  searchParams.forEach((value, key) => url.searchParams.set(key, value));

  const headers = new Headers();
  // Forward content-type and cookies
  const ct = request.headers.get("content-type");
  if (ct) headers.set("content-type", ct);
  const cookie = request.headers.get("cookie");
  if (cookie) headers.set("cookie", cookie);

  const init: RequestInit = {
    method: request.method,
    headers,
    redirect: "manual", // Don't follow redirects - pass them through
  };

  if (request.method !== "GET" && request.method !== "HEAD") {
    init.body = await request.text();
  }

  const upstream = await fetch(url.toString(), init);

  // For redirects, rewrite the Location header to be relative to frontend
  if (upstream.status >= 300 && upstream.status < 400) {
    const location = upstream.headers.get("location") || "";
    const responseHeaders = new Headers();

    // Forward Set-Cookie headers from backend
    const setCookies = upstream.headers.getSetCookie();
    for (const sc of setCookies) {
      responseHeaders.append("set-cookie", sc);
    }

    // If redirect is to backend's own paths (like /), pass through
    // If redirect is to an external URL (Keycloak), pass through as-is
    responseHeaders.set("location", location);

    return new Response(null, {
      status: upstream.status,
      headers: responseHeaders,
    });
  }

  // For non-redirect responses, forward body and Set-Cookie
  const responseHeaders = new Headers();
  const contentType = upstream.headers.get("content-type") || "";
  if (contentType) responseHeaders.set("content-type", contentType);

  const setCookies = upstream.headers.getSetCookie();
  for (const sc of setCookies) {
    responseHeaders.append("set-cookie", sc);
  }

  return new Response(upstream.body, {
    status: upstream.status,
    headers: responseHeaders,
  });
}

export const GET = proxy;
export const POST = proxy;
