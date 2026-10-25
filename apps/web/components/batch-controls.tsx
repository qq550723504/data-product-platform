"use client";
import {businessText,displayText} from '../lib/business-language.mjs';
import { useActionState,useEffect,useState } from "react";
import { registerBatch,type BatchState } from "@/app/batches/actions";
export function CommandButton({action,label,disabled=false}:{action:(state:BatchState,form:FormData)=>Promise<BatchState>;label:string;disabled?:boolean}){
  const [state,submit,pending]=useActionState(action,{message:""});return <form action={submit}><button className="button button-primary" disabled={pending||disabled}>{displayText(pending?"正在核对…":label)}</button>{displayText(state.message&&<p role="status">{businessText(state.message)}</p>)}</form>;
}
export function RegisterBatchForm(){const [state,action,pending]=useActionState(registerBatch,{message:""});const [key,setKey]=useState("");useEffect(()=>setKey(crypto.randomUUID()),[]);return <form action={action} className="detail-card"><h2>登记来源并创建批次</h2><p>固定 PostgreSQL 三表；原值进入 原始数据，清洗生成新版本。</p><input type="hidden" name="requestKey" value={key}/><label>批次名称 <input name="batchKey" required defaultValue="postgres-2025-03" pattern="[a-zA-Z0-9_-]{1,128}"/></label><button className="button button-primary" disabled={pending||!key}>登记来源并准备批次</button>{displayText(state.message&&<p role="alert">{businessText(state.message)}</p>)}</form>;}
