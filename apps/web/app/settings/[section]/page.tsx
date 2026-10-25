import {notFound} from 'next/navigation';
import {ManagementPage} from '@/components/management-page';
export default async function Page({params,searchParams}:{params:Promise<{section:string}>;searchParams:Promise<Record<string,string|string[]|undefined>>}){const {section}=await params;if(!['members','roles','audit','workspaces','users','organizations','logs','configuration'].includes(section))notFound();return <ManagementPage section={section} query={await searchParams}/>}
