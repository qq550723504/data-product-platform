import {businessText,displayText} from '../../lib/business-language.mjs';
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
 return <Modal title="演示页面上下文" onClose={onClose} className="agent-drawer"><div className="agent-drawer-content"><Note>这里展示的是本地示例。进入 Agent 工作台后，请选择系统中明确的成果版本和质量记录。</Note><div className="agent-context-card"><small>当前工作空间</small><strong>{businessText(workspace.name)}</strong><span>{businessText(workspace.id)}</span></div><label className="agent-version-field">成果版本<select aria-label="上下文版本" value={selected} onChange={e=>setSelected(e.target.value)}><option value="">请选择明确版本</option>{displayText(s.versions.map(v=><option key={v.id} value={v.id}>{businessText(v.id)} · {businessText(v.name)}</option>))}</select></label>{version?<div className="agent-context-card"><strong>{businessText(version.name)}</strong><span>{businessText(version.id)} · {businessText(version.layer)} · {displayText(version.rows)} 条合成记录</span><dl><dt>来源</dt><dd>{displayText(version.sources.join('、'))}</dd><dt>本地质量评测</dt><dd>{displayText(quality?quality.id+' · '+(quality.passed?'所选标准通过':quality.failures+' 条异常'):'尚未独立评测')}</dd></dl></div>:<p>选择版本后查看本地依据，不自动带入其他页面的对象。</p>}<button className="button" onClick={()=>{onClose();navigate('agent-workbench')}}>进入 Agent 工作台<ArrowRight size={17}/></button><div className="agent-trust"><ShieldCheck size={22}/><p>预览记录不会用于创建正式 Agent 任务。</p></div></div></Modal>;
}
