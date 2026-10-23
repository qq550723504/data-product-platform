import React from 'react';
import {ArrowRight,Info} from '@phosphor-icons/react';
export function Tag({children,tone='gray'}){return <span className={'tag '+tone}>{children}</span>}
export function Btn({children,onClick,secondary=false,quiet=false,disabled=false,...props}){return <button className={quiet?'text-button':secondary?'button secondary':'button'} onClick={onClick} disabled={disabled} {...props}>{children}{!quiet&&!secondary&&<ArrowRight size={18}/>}</button>}
export function Table({headers,rows,onRow,selected}){return <div className="table-wrap"><table><thead><tr>{headers.map(x=><th key={x}>{x}</th>)}</tr></thead><tbody>{rows.map((r,i)=><tr key={i} className={(onRow?'clickable ':'')+(selected===i?'selected':'')} tabIndex={onRow?0:undefined} onClick={onRow?()=>onRow(i):undefined} onKeyDown={e=>{if(onRow&&['Enter',' '].includes(e.key)){e.preventDefault();onRow(i)}}}>{r.map((c,j)=><td key={j}>{c}</td>)}</tr>)}</tbody></table>{!rows.length&&<div className="empty">当前空间没有匹配记录。</div>}</div>}
export function Head({crumb,title,description,action,badge}){return <><div className="breadcrumb">{crumb.split(' / ').map((p,i)=><React.Fragment key={i}>{i>0&&<span className="slash">/</span>}{p}</React.Fragment>)}</div><div className="page-heading"><div><h1>{title}{badge}</h1><p>{description}</p></div><div className="head-action">{action}</div></div></>}
export function Note({children}){return <div className="note"><Info size={18} weight="fill"/>{children}</div>}
export const components={Head,Table,Tag,Btn,Note};
export const tone=status=>['成功','已认证','通过','已发布','已验收','已归档','授权已提交','已批准','模拟传输完成'].includes(status)?'green':['失败','未通过','阻断','已阻断','模拟失败'].includes(status)?'red':'amber';
