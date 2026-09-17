import type { Metadata } from "next";
import { ConsoleShell } from "@/components/shell";
import "./globals.css";

export const metadata: Metadata = {
  title: "Data Product Platform · POC",
  description: "数据产品生产与治理平台 POC 控制台",
};

// 控制台在请求时读取 POC_WORKSPACE_ID。若允许预渲染，构建期（例如 CI 中该变量为空）
// 会把 "尚未配置" 分支固化成静态页面，之后即使 `next start` 时提供了真实 workspace，
// 服务端仍会返回这份静态 HTML。因此所有路由都必须按请求渲染。
export const dynamic = "force-dynamic";

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN">
      <body>
        <ConsoleShell>{children}</ConsoleShell>
      </body>
    </html>
  );
}
