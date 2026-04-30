import { NextRequest, NextResponse } from "next/server";

export function proxy(request: NextRequest) {
  const base = (process.env.JUNO_PAY_BASE_URL ?? "").replace(/\/+$/, "");
  if (!base) return NextResponse.next();

  const { pathname, search } = request.nextUrl;
  return NextResponse.rewrite(new URL(`${base}${pathname}${search}`));
}

export const config = {
  matcher: ["/admin", "/admin/:path*", "/v1/admin/:path*"],
};
