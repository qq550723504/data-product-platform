import type { Metadata } from "next";
import { ConsoleShell } from "@/components/shell";
import "./globals.css";

// Workspace and write permissions are deployment-time settings. Do not freeze
// SetupRequired at build time or switch static pages to dynamic after an action.
export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Data Product Platform · POC",
  description: "数据产品生产与治理平台 POC 控制台",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN">
      <body>
        <ConsoleShell>{children}</ConsoleShell>
      </body>
    </html>
  );
}
