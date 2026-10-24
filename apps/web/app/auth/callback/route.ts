import { NextRequest, NextResponse } from "next/server";
import { browserIAM } from "@/lib/iam-server";
import { iamEnabled } from "@/lib/iam-mode";
import { browserIdentityUnavailable } from "@/lib/iam-response";
import {
  checkBrowserRequest,
  cookieNames,
  cookieOptions,
} from "@/lib/iam-core";
export const runtime = "nodejs";
export const dynamic = "force-dynamic";
export async function GET(request: NextRequest) {
  if (!iamEnabled()) return new Response(null, { status: 404 });
  let response: NextResponse;
  try {
    const iam = await browserIAM();
    checkBrowserRequest(iam.config, request);
    const completed = await iam.callback(
      new URL(request.url),
      request.cookies.get(cookieNames.handshake)?.value ?? "",
      request.cookies.get(cookieNames.session)?.value,
    );
    response = NextResponse.redirect(
      new URL("/identity", iam.config.origin),
      303,
    );
    response.cookies.set(cookieNames.session, completed.key, {
      ...cookieOptions,
      expires: new Date(completed.expiresAt),
    });
  } catch {
    response = browserIdentityUnavailable();
  }
  response.cookies.set(cookieNames.handshake, "", {
    ...cookieOptions,
    maxAge: 0,
  });
  response.headers.set("Cache-Control", "no-store");
  response.headers.set("Referrer-Policy", "no-referrer");
  return response;
}
