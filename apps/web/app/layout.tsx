import type { Metadata } from "next";
import { ConsoleShell } from "@/components/shell";
import "./globals.css";

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
