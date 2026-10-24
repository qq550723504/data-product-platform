import CoreHistoryView from "@/components/core-history-view";
export default function Page(props:{searchParams:Promise<Record<string,string|string[]|undefined>>}){return <CoreHistoryView {...props}/>;}
