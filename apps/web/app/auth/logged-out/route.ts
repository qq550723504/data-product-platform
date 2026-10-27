import { NextRequest } from "next/server";
import { browserIAM } from "@/lib/iam-server";
import { iamEnabled } from "@/lib/iam-mode";
import { browserIdentityUnavailable } from "@/lib/iam-response";
import { checkBrowserRequest } from "@/lib/iam-core";
export const dynamic = "force-dynamic";
export async function GET(request: NextRequest) {
  if (!iamEnabled()) return new Response(null, { status: 404 });
  try {
    checkBrowserRequest((await browserIAM()).config, request);
    return new Response(
      `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>退出返回页</title></head>
<body><main>
<h1>退出返回页</h1>
<p>请在身份页确认本应用当前会话状态；其他应用的退出状态需分别确认。</p>
<p><a href="/identity">查看当前登录状态</a></p>
<p><a href="/auth/login">重新登录</a></p>
</main></body></html>`,
      {
        headers: {
          "Content-Type": "text/html; charset=utf-8",
          "Cache-Control": "no-store",
          "Referrer-Policy": "no-referrer",
          "X-Content-Type-Options": "nosniff",
          "Content-Security-Policy": "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'",
        },
      },
    );
  } catch {
    return browserIdentityUnavailable();
  }
}
