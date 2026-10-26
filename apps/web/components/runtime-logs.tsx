/* eslint-disable @next/next/no-html-link-for-pages -- Full GETs revalidate privileged reads; no router-prefetched records. */
import {RuntimeLogNavigation} from './runtime-log-navigation';
import {logServices,logLevels,logEvents,logHref,logListHref,type LogView,type LogSelection,type LogWindow,type LogEntry} from '../lib/runtime-logs';
const messages:Record<string,string>={LOG_SOURCE_UNCONFIGURED:'运行日志尚未接入',LOG_SOURCE_UNSUPPORTED:'来源暂不支持当前查询',AUTHENTICATION_REQUIRED:'登录状态不可用',LOG_ACCESS_DENIED:'没有当前空间的运行日志查看权限',INVALID_LOG_QUERY:'查询条件无效',LOG_QUERY_EXPIRED:'查询位置已失效',LOG_ENTRY_UNAVAILABLE:'指定运行记录当前不可见',LOG_ENTRY_CHANGED:'运行记录已变化，请重新查询',LOG_WINDOW_EXPIRED:'当前时间范围已不可查询',LOG_SOURCE_UNAVAILABLE:'运行记录暂时无法读取'};
function Time({value}:{value:string}){return <time dateTime={value}>{value.replace('T',' ').replace('Z',' UTC')}</time>}
function Filters({selection:s}:{selection:LogSelection}){return <form action="/settings/logs" method="get" className="management-filters runtime-log-filters" aria-label="运行日志查询">
 <label>开始时间（UTC）<input name="start" aria-label="开始时间（UTC）" defaultValue={s.start} required/></label><label>结束时间（UTC）<input name="end" aria-label="结束时间（UTC）" defaultValue={s.end} required/></label>
 <label>服务<select name="service" aria-label="服务" defaultValue={s.service??''}><option value="">全部服务</option>{Object.entries(logServices).map(([v,label])=><option key={v} value={v}>{label}</option>)}</select></label>
 <label>级别<select name="level" aria-label="级别" defaultValue={s.level??''}><option value="">全部级别</option>{Object.entries(logLevels).map(([v,label])=><option key={v} value={v}>{label}</option>)}</select></label>
 {s.from&&<input type="hidden" name="from" value={s.from}/>}<button type="submit" className="button">查询</button><a href={'/settings/logs'+(s.from?'?from=configuration':'')}>清除条件</a>
 </form>}
function Evidence({window:w}:{window:LogWindow}){return <details className="management-evidence"><summary>查看查询依据</summary><p>记录时间表示来源观察到记录的时间；发生时间可能未提供。</p><p>查询时点：<Time value={w.asOf}/></p><p>来源保留与晚到记录可能影响可见结果。</p></details>}
function Facts({item:e}:{item:LogEntry}){return <dl className="management-member-facts"><dt>记录时间</dt><dd><Time value={e.recordedAt}/></dd><dt>发生时间</dt><dd>{e.occurredAt?<Time value={e.occurredAt}/>:'未提供'}</dd><dt>服务</dt><dd>{logServices[e.service]}</dd><dt>级别</dt><dd>{logLevels[e.level]}</dd><dt>摘要</dt><dd>{logEvents[e.eventCode]}</dd></dl>}
export function RuntimeLogs({view}:{view:LogView}){
 const {selection:s,result,detail,code}=view;
 const title=s?.entryRef?'运行记录详情':'运行日志',loaded=result??detail;
 return <><RuntimeLogNavigation/><div className="breadcrumb">系统设置 / 运行日志{s?.entryRef?' / 运行记录详情':''}</div><div className="page-heading"><div><h1>{title}</h1><p>查看当前空间的服务运行记录。</p></div></div>
 {s?.from&&<a className="management-back" href="/settings/configuration"><span aria-hidden="true">← </span>返回基础配置</a>}
 {s&&!s.entryRef&&<Filters selection={s}/>}
 {code?<section role="status" className="management-state"><h2>{messages[code]??messages.LOG_SOURCE_UNAVAILABLE}</h2>{code==='LOG_SOURCE_UNCONFIGURED'&&<p>尚未配置当前空间的运行日志来源。</p>}
 {code==='AUTHENTICATION_REQUIRED'?<a href="/identity">重新核验身份</a>:<a href="/settings">返回设置总览</a>}
 {code==='INVALID_LOG_QUERY'&&!s&&<a className="management-back" href="/settings/logs">清除条件</a>}
 {s?.entryRef&&<a className="management-back" href={logListHref(s,s.focus)}>返回运行日志</a>}
 {s&&!['LOG_ACCESS_DENIED','AUTHENTICATION_REQUIRED'].includes(code)&&<a className="management-back" href={logHref(s,{cursor:undefined,entryRef:undefined,focus:undefined})}>按当前条件重新查询</a>}
 </section>:loaded&&s&&<section data-runtime-log-results>
 {loaded.source==='SYNTHETIC_FIXTURE'&&<p className="management-source">合成验收数据 · 真实来源未配置</p>}
 {detail?<section className="management-detail" aria-label="运行记录详情"><a className="management-back" href={logListHref(s,detail.data.item.rowKey)}><span aria-hidden="true">← </span>返回运行日志</a><Facts item={detail.data.item}/></section>:result&&<>
 <div className="table-wrap"><table aria-label="运行日志"><thead><tr><th>记录时间</th><th>级别</th><th>服务</th><th>摘要</th><th>操作</th></tr></thead><tbody>{result.data.items.map(e=><tr key={e.rowKey} id={'log-'+e.rowKey} tabIndex={-1}><td><Time value={e.recordedAt}/></td><td>{logLevels[e.level]}</td><td>{logServices[e.service]}</td><td>{logEvents[e.eventCode]}</td><td><a href={logHref(s,{entryRef:e.entryRef,focus:e.rowKey})}>查看记录</a></td></tr>)}</tbody></table></div>
 {!result.data.items.length&&<p className="empty">当前条件下没有运行记录</p>}
 {s.focus&&!result.data.items.some(e=>e.rowKey===s.focus)&&<p role="status">原记录当前不可见</p>}
 <nav className="pagination" aria-label="运行日志分页"><a href={logHref(s,{cursor:undefined,focus:undefined})}>返回查询首页</a>{result.data.next?<a href={logHref(s,{cursor:result.data.next,focus:undefined})}>更早记录</a>:<span>已到末页</span>}</nav></>}
 <Evidence window={loaded.data.window}/></section>}
 </>;
}
