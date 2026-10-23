// Presentation state only; not a tenant or IAM authorization implementation.
export const workspaceRoles=['空间管理员','空间开发者','空间观察员'];
export const initialWorkspaces={currentId:'ws-enterprise',items:[
 {id:'ws-enterprise',name:'企业经营数据',description:'企业经营数据治理与成果交付演示',status:'启用',kind:'enterprise',owner:'USR-01',members:[{userId:'USR-01',role:'空间管理员'},{userId:'USR-02',role:'空间开发者'},{userId:'USR-03',role:'空间观察员'}]},
 {id:'ws-park',name:'园区能耗分析',description:'独立的园区用电分析样例',status:'启用',kind:'park',owner:'USR-01',members:[{userId:'USR-01',role:'空间管理员'},{userId:'USR-03',role:'空间观察员'}]}
]};
const check=(ok,message)=>{if(!ok)throw Error(message)};
export function switchWorkspace(s,id){const w=s.items.find(x=>x.id===id);check(w?.status==='启用','只能进入启用的空间');check(w.members.some(m=>m.userId==='USR-01'),'当前演示成员未加入此空间');return {...s,currentId:id}}
export function saveWorkspace(s,draft){
 const name=draft.name.trim();check(name.length>0,'请填写空间名称');check(!s.items.some(x=>x.id!==draft.id&&x.name.toLowerCase()===name.toLowerCase()),'空间名称已存在');
 const old=s.items.find(x=>x.id===draft.id);const id=old?.id||'ws-'+String(s.items.length+1).padStart(3,'0');
 if(old&&draft.owner)check(old.members.some(m=>m.userId===draft.owner&&m.role==='空间管理员'),'负责人须为现有空间管理员');
 const w=old?{...old,name,description:draft.description.trim(),owner:draft.owner||old.owner}:{id,name,description:draft.description.trim(),status:'启用',kind:'empty',owner:'USR-01',members:[{userId:'USR-01',role:'空间管理员'}]};
 return {...s,items:old?s.items.map(x=>x.id===id?w:x):[...s.items,w]};
}
export function archiveWorkspace(s,id){check(id!==s.currentId,'请先切换到其他空间，再归档此空间');const w=s.items.find(x=>x.id===id);check(w,'空间不存在');return {...s,items:s.items.map(x=>x.id===id?{...x,status:x.status==='启用'?'已归档':'启用'}:x)}}
export function saveMember(s,id,member){
 check(workspaceRoles.includes(member.role),'请选择空间角色');const w=s.items.find(x=>x.id===id);check(w&&w.status==='启用','空间已归档，无法调整成员');
 const members=w.members.some(m=>m.userId===member.userId)?w.members.map(m=>m.userId===member.userId?{...member}:m):[...w.members,{...member}];
 check(members.some(m=>m.role==='空间管理员'),'至少保留一名空间管理员');check(members.some(m=>m.userId===w.owner&&m.role==='空间管理员'),'空间负责人必须保留管理员角色');
 return {...s,items:s.items.map(x=>x.id===id?{...x,members}:x)};
}
export function removeMember(s,id,userId){const w=s.items.find(x=>x.id===id);check(w&&w.status==='启用','空间已归档');check(w.owner!==userId,'请保留空间负责人');return {...s,items:s.items.map(x=>x.id===id?{...x,members:x.members.filter(m=>m.userId!==userId)}:x)}}
export function dataKey(workspaceId,key){return ['admin-state','profile'].includes(key)?'platform:'+key:workspaceId+':'+key}
