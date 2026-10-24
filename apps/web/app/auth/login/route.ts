import { NextRequest, NextResponse } from "next/server";
import { browserIAM } from "@/lib/iam-server";
import { iamEnabled } from "@/lib/iam-mode";
import { browserIdentityUnavailable } from "@/lib/iam-response";
import {workspaces,sealedWorkspace,workspaceCookie} from '@/lib/iam-workspace';
import {
  checkBrowserRequest,
  readCSRFForm,
  cookieNames,
  cookieOptions,
  equal,
  randomKey,
  oidcFormPolicy,
} from "@/lib/iam-core";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";
const safeHeaders = {
  "Cache-Control": "no-store",
  // no-referrer makes Chromium's form POST Origin null, which our gate rejects.
  // same-origin preserves that gate while omitting cross-origin Referer headers.
  "Referrer-Policy": "same-origin",
  "Content-Security-Policy":
    "default-src 'none'; form-action 'self'; frame-ancestors 'none'",
  "X-Content-Type-Options": "nosniff",
};
export async function GET(request: NextRequest) {
  if (!iamEnabled()) return new Response(null, { status: 404 });
  try {
    const query=request.nextUrl.searchParams;
    if([...query.keys()].some(k=>k!=="workspaceId")||query.getAll("workspaceId").length>1)throw new Error();
    const chosen=query.get("workspaceId")??undefined;
    if(chosen&&!workspaces().some(v=>v.id===chosen))throw new Error();
    const iam = await browserIAM(chosen);
    checkBrowserRequest(iam.config, request);
    const csrf = randomKey();
    const response = new NextResponse(
      `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>统一身份登录</title><h1>统一身份登录</h1><p>通过已配置的 OIDC 身份源确认本人及工作空间资格。此验收入口仅提供只读责任预检。</p><p>当前工作空间：${iam.config.workspaceId}</p><form method="post" action="/auth/login"><input type="hidden" name="csrf" value="${csrf}"><button type="submit">使用统一身份登录</button></form><p>切换验收空间：</p>${workspaces().map(v=>`<p><a href="/auth/login?workspaceId=${v.id}">${v.id}</a></p>`).join('')}</html>`,
      {
        headers: { ...safeHeaders, "Content-Type": "text/html; charset=utf-8",
          "Content-Security-Policy": oidcFormPolicy(iam.config.authorizationEndpoint) },
      },
    );
    response.cookies.set(cookieNames.csrf, csrf, {
      ...cookieOptions,
      maxAge: 300,
    });
    response.cookies.set(workspaceCookie,sealedWorkspace(iam.config.workspaceId),{...cookieOptions,maxAge:28800});
    return response;
  } catch {
    return browserIdentityUnavailable(503);
  }
}
export async function POST(request: NextRequest) {
  if (!iamEnabled()) return new Response(null, { status: 404 });
  try {
    const iam = await browserIAM();
    checkBrowserRequest(iam.config, request, true);
    const csrf = await readCSRFForm(request);
    if (!equal(csrf, request.cookies.get(cookieNames.csrf)?.value ?? ""))
      throw new Error();
    const started = await iam.start();
    // Only one browser-bound handshake cookie is active. Previous states expire.
    const response = NextResponse.redirect(started.url, 303);
    for (const [key, value] of Object.entries(safeHeaders))
      response.headers.set(key, value);
    response.headers.set("Content-Security-Policy", oidcFormPolicy(iam.config.authorizationEndpoint));
    response.cookies.set(cookieNames.csrf, "", { ...cookieOptions, maxAge: 0 });
    response.cookies.set(cookieNames.handshake, started.key, {
      ...cookieOptions,
      maxAge: 300,
    });
    return response;
  } catch {
    return browserIdentityUnavailable();
  }
}
