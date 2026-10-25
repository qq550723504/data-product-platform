"use client";
import {businessText,displayText} from '../lib/business-language.mjs';
import {useActionState,useEffect,useState} from "react";
import {requestSchema,handleSchema} from "@/app/resources/schema-actions";

export function SchemaScanForm({resourceId}:{resourceId:string}){
 const [state,action,pending]=useActionState(requestSchema.bind(null,resourceId),{message:""});
 const [key,setKey]=useState("");useEffect(()=>setKey(crypto.randomUUID()),[state.requestId]);
 return <form action={action}><input type="hidden" name="requestKey" value={key}/><button className="button button-primary" disabled={!key||pending}>{businessText(pending?"提交中…":"请求新的原生 schema 扫描")}</button>{displayText(state.message&&<p role="status">{businessText(state.message)}</p>)}</form>;
}
export function SchemaHandlingForm({resourceId,observationId}:{resourceId:string;observationId:string}){
 const [state,action,pending]=useActionState(handleSchema.bind(null,resourceId,observationId),{message:""});
 const [key,setKey]=useState("");useEffect(()=>setKey(crypto.randomUUID()),[]);
 return <form action={action}><input type="hidden" name="requestKey" value={key}/><label>数据结构 异常处理理由 <input name="reason" required maxLength={1000}/></label><button className="button" disabled={!key||pending}>记录开始处理</button>{displayText(state.message&&<p role="status">{businessText(state.message)}</p>)}</form>;
}
