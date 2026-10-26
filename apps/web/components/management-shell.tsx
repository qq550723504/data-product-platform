"use client";
import {GearSix,Users,ShieldCheck,Clock,Buildings,TreeStructure,SlidersHorizontal,FileText} from '@phosphor-icons/react';
const icons=[GearSix,Buildings,Users,Users,TreeStructure,ShieldCheck,Clock,FileText,SlidersHorizontal];
import {usePathname} from "next/navigation";
import {ManagementShellView} from "./management-shell-view";
export function ManagementShell({children,identityEnabled=false}:{children:React.ReactNode;identityEnabled?:boolean}){return <ManagementShellView path={usePathname()} icons={icons} identityEnabled={identityEnabled}>{children}</ManagementShellView>}
