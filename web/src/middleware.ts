import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";
import logger from "@/lib/logger";

export function middleware(request: NextRequest) {
  const apiUrl = process.env.API_URL_DEV;

  // Only proxy if API_URL_DEV is set (local dev proxy)
  if (!apiUrl) {
    return NextResponse.next();
  }

  const { pathname } = request.nextUrl;

  // Proxy /api/* and /auth/* to API
  if (pathname.startsWith("/api/") || pathname.startsWith("/auth/")) {
    const url = new URL(pathname + request.nextUrl.search, apiUrl);
    logger.debug({ msg: "dev proxy rewrite", path: pathname, target: url.toString() });
    return NextResponse.rewrite(url);
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/api/:path*", "/auth/:path*"],
};
