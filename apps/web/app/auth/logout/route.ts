import { NextRequest, NextResponse } from "next/server";
import { browserIAM } from "@/lib/iam-server";
import { iamEnabled } from "@/lib/iam-mode";
import { browserIdentityUnavailable } from "@/lib/iam-response";
import {
  checkBrowserRequest,
  readCSRFForm,
  cookieNames,
  cookieOptions,
  oidcFormPolicy,
} from "@/lib/iam-core";
export const runtime = "nodejs";
export const dynamic = "force-dynamic";
export async function GET(request: NextRequest) {
  if (!iamEnabled()) return new Response(null, { status: 404 });
  try {
    const iam = await browserIAM();
    checkBrowserRequest(iam.config, request);
    const csrf = await iam.logoutForm(
      request.cookies.get(cookieNames.session)?.value ?? "",
    );
    return new Response(
      `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>退出</title><form method="post" action="/auth/logout"><input type="hidden" name="csrf" value="${csrf}"><button type="submit">退出本应用</button></form></html>`,
      {
        headers: {
          "Content-Type": "text/html; charset=utf-8",
          "Cache-Control": "no-store",
          "Referrer-Policy": "same-origin",
          "Content-Security-Policy": oidcFormPolicy(iam.config.logoutEndpoint),
        },
      },
    );
  } catch {
    return browserIdentityUnavailable();
  }
}
export async function POST(request: NextRequest) {
  if (!iamEnabled()) return new Response(null, { status: 404 });
  try {
    const iam = await browserIAM();
    checkBrowserRequest(iam.config, request, true);
    const csrf = await readCSRFForm(request);
    const target = await iam.logout(
      request.cookies.get(cookieNames.session)?.value ?? "",
      csrf,
    );
    const response = NextResponse.redirect(target, 303);
    response.cookies.set(cookieNames.session, "", {
      ...cookieOptions,
      maxAge: 0,
    });
    response.headers.set("Cache-Control", "no-store");
    response.headers.set("Referrer-Policy", "same-origin");
    response.headers.set("Content-Security-Policy", oidcFormPolicy(iam.config.logoutEndpoint));
    return response;
  } catch {
    return browserIdentityUnavailable();
  }
}
