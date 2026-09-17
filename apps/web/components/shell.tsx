"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const navigation = [
  { href: "/", label: "工作台", mark: "W" },
  { href: "/resources", label: "数据资源", mark: "R" },
  { href: "/datasets", label: "数据集", mark: "D" },
  { href: "/production", label: "数据生产", mark: "P" },
  { href: "/reviews", label: "实体审核", mark: "E" },
  { href: "/products", label: "数据产品", mark: "O" },
  { href: "/evidence", label: "证据中心", mark: "A" },
];

function isActive(pathname: string, href: string): boolean {
  if (href === "/") return pathname === "/";
  return pathname === href || pathname.startsWith(`${href}/`);
}

export function ConsoleShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <div className="console-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">DP</div>
          <div>
            <strong>Data Product</strong>
            <span>可信生产控制台</span>
          </div>
        </div>

        <nav className="nav-list" aria-label="主导航">
          {navigation.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className={`nav-item ${isActive(pathname, item.href) ? "active" : ""}`}
            >
              <span className="nav-mark">{item.mark}</span>
              <span>{item.label}</span>
            </Link>
          ))}
        </nav>

        <div className="sidebar-note">
          <span className="eyebrow">POC · Enterprise Activity</span>
          <p>Core 负责业务事实与版本历史；外部引擎只作为可替换执行能力。</p>
        </div>
      </aside>

      <main className="main-area">
        <header className="topbar">
          <div>
            <span className="eyebrow">Data Product Platform</span>
            <strong>园区企业经营活跃度参考链路</strong>
          </div>
          <div className="environment-pill">POC</div>
        </header>
        <div className="page-container">{children}</div>
      </main>
    </div>
  );
}
