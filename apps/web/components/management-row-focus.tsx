'use client';
import {useEffect} from 'react';

// A hash only restores position in an authorized rendered list.
// It never fetches records, changes scope, or restores browser-cached data.
export function ManagementRowFocus({prefix}:{prefix:'member-'|'user-'|'log-'}){
 useEffect(()=>{
  const rowFor=(id:unknown)=>{
   if(typeof id!=='string'||!id.startsWith(prefix))return null;
   const row=document.getElementById(id);
   return row?.tagName==='TR'&&row.closest('#management-main')?row:null;
  };
  const focus=()=>{
   let id:unknown;try{id=decodeURIComponent(location.hash.slice(1))}catch{return}
   rowFor(id)?.focus();
  };
  const remember=(event:MouseEvent)=>{
   const link=event.target instanceof Element?event.target.closest('a[href]'):null,row=link?.closest('tr');
   // Reuse the explicit return-link fragment on this list's history entry.
   // Back still makes a fresh authorized GET; no records enter history state.
   if(row&&rowFor(row.id)===row)history.replaceState(history.state,'','#'+encodeURIComponent(row.id));
  };
  document.addEventListener('click',remember,true);
  focus();window.addEventListener('hashchange',focus);
  return ()=>{document.removeEventListener('click',remember,true);window.removeEventListener('hashchange',focus)};
 },[prefix]);
 return null;
}
