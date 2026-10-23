import React,{useState} from 'react';
import {ArrowRight,ShieldCheck} from '@phosphor-icons/react';
import {usePipeline} from './PipelineContext';
import {useWorkspace} from './WorkspaceContext';
import {Modal} from './DemoPages';
import {Note} from './ui';
import {qualityFor} from './lifecycle-model.mjs';
export function AgentContextSidebar({navigate,onClose}){
 const [s]=usePipeline();const {workspace}=useWorkspace();const [selected,setSelected]=useState('');
 const version=s.versions.find(v=>v.id===selected);const quality=version&&qualityFor(s,version.id);
 return <Modal title="演示页面上下文" onClose={onClose} className="agent-drawer"><div className="agent-drawer-content"><Note>这里是本地合成对象，不能作为 Core 的真实版本或质量依据。进入 Agent 后需选择服务端明确版本。</Note><div className="agent-context-card"><small>演示工作空间</small><strong>{workspace.name}</strong><span>{workspace.id}</span></div><label className="agent-version-field">演示成果版本<select aria-label="上下文版本" value={selected} onChange={e=>setSelected(e.target.value)}><option value="">请选择明确版本</option>{s.versions.map(v=><option key={v.id} value={v.id}>{v.id} · {v.name}</option>)}</select></label>{version?<div className="agent-context-card"><strong>{version.name}</strong><span>{version.id} · {version.layer} · {version.rows} 条合成记录</span><dl><dt>来源</dt><dd>{version.sources.join('、')}</dd><dt>本地质量评测</dt><dd>{quality?quality.id+' · '+(quality.passed?'所选标准通过':quality.failures+' 条异常'):'尚未独立评测'}</dd></dl></div>:<p>选择版本后查看本地依据，不自动带入其他页面的对象。</p>}<button className="button" onClick={()=>{onClose();navigate('agent-workbench')}}>进入 Core Agent 工作台<ArrowRight size={17}/></button><div className="agent-trust"><ShieldCheck size={22}/><p>不会把演示版本 ID 提交到后端，也不会创建一份并行的模拟 Agent 委托。</p></div></div></Modal>;
}
