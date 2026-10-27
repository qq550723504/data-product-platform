"use client";
import {businessText,displayText} from '../lib/business-language.mjs';
import { useActionState,useEffect,useState } from "react";
import type { BatchState } from "@/app/batches/actions";
export function RemediationForm({action,label,batches,executionLinkId,requestKey,disabled=false}:{action:(s:BatchState,f:FormData)=>Promise<BatchState>;label:string;batches?:Array<{id:string;name:string}>;executionLinkId?:string;requestKey?:string;disabled?:boolean}){
 const [state,submit,pending]=useActionState(action,{message:""});const [key,setKey]=useState(requestKey??"");
 useEffect(()=>{if(!requestKey)setKey(crypto.randomUUID());},[requestKey]);
 return <form action={submit} className="detail-card">
  <input type="hidden" name="requestKey" value={requestKey??key}/>
  {displayText(executionLinkId?<input type="hidden" name="executionLinkId" value={executionLinkId}/>:<label>处理理由 <input name="reason" required maxLength={1000} /></label>)}
  {batches&&<label>包含该 原始数据 的接纳批次 <select name="batchId" required>{displayText(batches.map(b=><option key={b.id} value={b.id}>{businessText(b.name)}</option>))}</select></label>}
  <button className="button button-primary" disabled={disabled||pending||!(requestKey??key)}>{displayText(pending?"正在核对…":label)}</button>
  {displayText(state.message&&<p role="status">{businessText(state.message)}</p>)}
 </form>;
}
