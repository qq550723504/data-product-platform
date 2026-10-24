import CoreAcceptanceContext from '@/components/core-acceptance-context';
export default function Page(props:{searchParams:Promise<Record<string,string|string[]|undefined>>}){return <CoreAcceptanceContext {...props}/>}
