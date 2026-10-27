import { NextResponse } from "next/server";

// Chromium may block a top-level plain-text failure in this HTTPS pilot.
// Keep the denial status and use one static document for every auth entrypoint.
export function browserIdentityUnavailable(status: 403 | 503 = 403) {
  return new NextResponse(
    `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>登录未完成</title></head><body><main><h1>登录未完成</h1><p>当前登录无法进入本应用。请联系管理员确认账号准入，或重新登录。</p><p><a href="/auth/login">返回登录页</a></p><p><a href="/identity">查看当前登录状态</a></p></main></body></html>`,
    {
      status,
      headers: {
        "Content-Type": "text/html; charset=utf-8",
        "Cache-Control": "no-store",
        "Referrer-Policy": "no-referrer",
        "X-Content-Type-Options": "nosniff",
        "Content-Security-Policy": "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'",
      },
    },
  );
}
