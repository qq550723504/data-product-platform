import {redirect} from 'next/navigation';
export default async function Page({params}:{params:Promise<{id:string;releaseId:string}>}){
 const {id,releaseId}=await params;
 redirect('/core/release-trace?'+new URLSearchParams({productId:id,releaseId}));
}
