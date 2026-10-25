import {displayText} from '../lib/business-language.mjs';
import type { Metadata } from "next";
import { ConsoleShell } from "@/components/shell";
import {AcceptanceIdentityShell} from '@/components/acceptance-identity-shell';
import './identity.css';
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
        {process.env.BROWSER_IAM_MODE==='oidc'&&process.env.BROWSER_IAM_PROFILE==='business-acceptance'?<AcceptanceIdentityShell>{displayText(children)}</AcceptanceIdentityShell>:<ConsoleShell syntheticPreview={process.env.GOVERNANCE_PREVIEW_MODE==='synthetic-readonly'} managementIdentity={process.env.BROWSER_IAM_MODE==='oidc'&&process.env.BROWSER_IAM_PROFILE==='platform-management'}>{displayText(children)}</ConsoleShell>}
      </body>
    </html>
  );
}
