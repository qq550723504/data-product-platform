"use client";
import dynamic from "next/dynamic";
import {usePathname} from "next/navigation";
import {ConnectedApp} from "./governance-demo/App";
// Only the synthetic demo initializes state from browser-local storage.
const DemoApp=dynamic(()=>import("./governance-demo/App").then(m=>m.App),{ssr:false,loading:()=> <p role="status" className="demo-loading">正在打开数据治理工作空间…</p>});
export function ConsoleShell({children}:{children:React.ReactNode}){
 const pathname=usePathname();
 if(pathname!=="/"&&pathname!=="/demo")return <ConnectedApp liveAgent={pathname==="/agent"||pathname.startsWith("/agent/")}>{children}</ConnectedApp>;
 return <DemoApp liveAgent={false}>{children}</DemoApp>;
}
