import React,{useContext,useState,useEffect} from 'react';
import {DemoContext} from './demo-context.mjs';
import {useWorkspace} from './WorkspaceContext';
import {dataKey} from './workspace-model.mjs';
import {seedWorkspacePipeline} from './workspace-seeds.mjs';
import {demoStorageKeys,readDemoSession,saveDemoSession} from './demo-session.mjs';
export function DemoProvider({children}){
 const [data,setData]=useState(()=>readDemoSession(demoStorageKeys.data,{}));
 const [storageAvailable,setStorageAvailable]=useState(null);
 useEffect(()=>{setStorageAvailable(saveDemoSession(demoStorageKeys.data,data))},[data]);
 const update=(key,value)=>setData(old=>({...old,[key]:typeof value==='function'?value(old[key]):value}));
 return <DemoContext.Provider value={{data,update,storageAvailable}}>{children}</DemoContext.Provider>
}
export function DemoStorageWarning(){const demo=useContext(DemoContext);const workspace=useWorkspace();return demo?.storageAvailable===false||workspace?.storageAvailable===false?<div className="prototype-notice" role="alert">本地会话存储不可用：当前修改仅在内存中，刷新会丢失；请勿作为已保存证据。</div>:null}
export function useDemo(key,initial){const {data,update}=useContext(DemoContext);const {workspace}=useWorkspace();const scoped=dataKey(workspace.id,key);return [data[scoped]??initial,value=>update(scoped,old=>typeof value==='function'?value(old??initial):value)]}
export function useBusinessActivities(){const {data}=useContext(DemoContext);const {registry}=useWorkspace();return registry.items.flatMap(w=>(data[dataKey(w.id,'pipeline-state')]??seedWorkspacePipeline(w)).events.map(e=>({...e,workspaceId:w.id,workspaceName:w.name})))}

// Global account/settings examples stay independent of the selected business workspace.
export function useGlobalDemo(key,initial){const {data,update}=useContext(DemoContext);const scoped="global:"+key;return [data[scoped]??initial,value=>update(scoped,old=>typeof value==="function"?value(old??initial):value)]}
