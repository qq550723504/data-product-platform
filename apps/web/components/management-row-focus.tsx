'use client';
import {useEffect} from 'react';

// A hash only restores keyboard position in an already-authorized rendered list.
// It never fetches records, changes scope, or restores browser-cached data.
export function ManagementRowFocus({prefix}:{prefix:'member-'|'user-'|'log-'}){
 useEffect(()=>{
  const focus=()=>{
   let id:string;try{id=decodeURIComponent(location.hash.slice(1))}catch{return}
   if(!id.startsWith(prefix))return;
   const row=document.getElementById(id);
   if(row?.tagName==='TR'&&row.closest('#management-main'))row.focus();
  };
  focus();window.addEventListener('hashchange',focus);
  return ()=>window.removeEventListener('hashchange',focus);
 },[prefix]);
 return null;
}
