import CoreDeliveryHistory from '@/components/core-delivery-history';
export default function Page(props:{searchParams:Promise<Record<string,string|string[]|undefined>>}){return <CoreDeliveryHistory {...props}/>}
