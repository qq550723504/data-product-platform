import CoreQualityView from "@/components/core-quality-view";
export default function Page(props: {searchParams: Promise<Record<string,string|string[]|undefined>>}) {return <CoreQualityView {...props}/>;}
