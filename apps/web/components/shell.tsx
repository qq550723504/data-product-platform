"use client";
import dynamic from "next/dynamic";
import {usePathname} from "next/navigation";
// Browser-local demo state never becomes input to the API-backed Agent controller.
const GovernanceApp=dynamic(()=>import("./governance-demo/App").then(m=>m.App),{ssr:false,loading:()=> <p role="status" className="demo-loading">正在打开数据治理工作空间…</p>});
export function ConsoleShell({children}:{children:React.ReactNode}){
 const pathname=usePathname();
 return <GovernanceApp liveCatalog={pathname.startsWith("/core/")} liveAgent={pathname==="/agent"||pathname.startsWith("/agent/")}>{children}</GovernanceApp>;
}
