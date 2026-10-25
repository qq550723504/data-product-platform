import React,{createContext,useContext,useState,useEffect} from 'react';
import {initialWorkspaces,switchWorkspace} from './workspace-model.mjs';
import {demoStorageKeys,readDemoSession,saveDemoSession} from './demo-session.mjs';
const Context=createContext(null);
export function WorkspaceProvider({children}){const [registry,setRegistry]=useState(()=>readDemoSession(demoStorageKeys.registry,initialWorkspaces));const [storageAvailable,setStorageAvailable]=useState(null);useEffect(()=>{setStorageAvailable(saveDemoSession(demoStorageKeys.registry,registry))},[registry]);const workspace=registry.items.find(w=>w.id===registry.currentId);const switchTo=id=>{setRegistry(s=>switchWorkspace(s,id));location.hash='home'};return <Context.Provider value={{registry,setRegistry,workspace,switchTo,storageAvailable}}>{children}</Context.Provider>}
export function useWorkspace(){return useContext(Context)}
