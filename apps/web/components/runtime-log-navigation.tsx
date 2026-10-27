'use client';
import {useEffect,useState,type ReactNode} from 'react';
import {flushSync} from 'react-dom';
import {runtimeLogBrowserGuard} from '../lib/runtime-logs';
// Keep history clearing inside React ownership, including server-rendered children.
export function RuntimeLogNavigation({children}:{children:ReactNode}){
 const [visible,setVisible]=useState(true);
 useEffect(()=>runtimeLogBrowserGuard(()=>flushSync(()=>setVisible(false))),[]);
 return visible?children:null;
}
