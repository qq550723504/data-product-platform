import {displayText} from '../../lib/business-language.mjs';
import React,{createContext,useContext,useState,useEffect} from 'react';
import {initialWorkspaces,switchWorkspace} from './workspace-model.mjs';
import {demoStorageKeys,readDemoSession,saveDemoSession} from './demo-session.mjs';
const Context=createContext(null);
export function WorkspaceProvider({children,settingsInteractionDemo=false}){const [registry,setRegistry]=useState(()=>readDemoSession(demoStorageKeys.registry,initialWorkspaces));const [storageAvailable,setStorageAvailable]=useState(null);useEffect(()=>{setStorageAvailable(saveDemoSession(demoStorageKeys.registry,registry))},[registry]);const workspace=registry.items.find(w=>w.id===registry.currentId);const switchTo=id=>{setRegistry(s=>switchWorkspace(s,id));location.hash=settingsInteractionDemo?'settings':'home'};return <Context.Provider value={{registry,setRegistry,workspace,switchTo,storageAvailable,switchTargetLabel:settingsInteractionDemo?'设置总览':'空间首页'}}>{displayText(children)}</Context.Provider>}
export function useWorkspace(){return useContext(Context)}
