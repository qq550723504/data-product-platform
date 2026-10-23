import type { Metadata } from "next";
import { ConsoleShell } from "@/components/shell";
import "./globals.css";
import "@/components/governance-demo/styles.css";
import "@/components/governance-demo/agent.css";
import "@/components/governance-demo/integration.css";

// Workspace and write permissions are deployment-time settings. Do not freeze
// SetupRequired at build time or switch static pages to dynamic after an action.
export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "数据治理平台",
  description: "数据治理工作空间、数据开发、资产管理与 Agent 工作台",
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
