"use client";
import {displayText} from '../lib/business-language.mjs';
import dynamic from "next/dynamic";
import {usePathname} from "next/navigation";
import {ConnectedApp} from "./governance-demo/App";
import {ManagementShell} from './management-shell';
// Only the synthetic demo initializes state from browser-local storage.
const DemoApp=dynamic(()=>import("./governance-demo/App").then(m=>m.App),{ssr:false,loading:()=> <p role="status" className="demo-loading">正在打开数据治理工作空间…</p>});
export function ConsoleShell({children,syntheticPreview=false,managementIdentity=false}:{children:React.ReactNode;syntheticPreview?:boolean;managementIdentity?:boolean}){
 const pathname=usePathname();
 if(pathname==='/settings'||pathname.startsWith('/settings/')||managementIdentity&&pathname==='/identity')return <ManagementShell identityEnabled={managementIdentity}>{displayText(children)}</ManagementShell>;
 if(pathname!=="/"&&pathname!=="/demo")return <ConnectedApp syntheticPreview={syntheticPreview} liveAgent={pathname==="/agent"||pathname.startsWith("/agent/")}>{displayText(children)}</ConnectedApp>;
 return <DemoApp syntheticPreview={syntheticPreview} liveAgent={false}>{displayText(children)}</DemoApp>;
}
