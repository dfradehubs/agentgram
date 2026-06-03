import { NextRequest, NextResponse } from "next/server";
import { register, sseErrors, ttfb, backgroundTransfers, pdfExports } from "@/lib/metrics";

export async function GET() {
  const metrics = await register.metrics();
  return new NextResponse(metrics, {
    headers: { "Content-Type": register.contentType },
  });
}

export async function POST(request: NextRequest) {
  try {
    const body = await request.json();
    const { name, labels, value } = body;

    switch (name) {
      case "sse_error":
        sseErrors.labels(labels).inc(value ?? 1);
        break;
      case "ttfb":
        ttfb.labels(labels).observe(value);
        break;
      case "background_transfer":
        backgroundTransfers.labels(labels).inc(value ?? 1);
        break;
      case "pdf_export":
        pdfExports.labels(labels).inc(value ?? 1);
        break;
      default:
        return NextResponse.json({ error: "unknown metric" }, { status: 400 });
    }

    return NextResponse.json({ ok: true });
  } catch {
    return NextResponse.json({ error: "invalid request" }, { status: 400 });
  }
}
