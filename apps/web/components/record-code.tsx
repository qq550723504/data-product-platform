/** Internal fixture codes stay available for traceability without leading the UI. */
export function RecordCode({value}:{value?:string|null}){
 if(!value)return <span>未提供编号</span>;
 return /(?:^|_)(?:FIXTURE|TEST|BROWSER)(?:_|$)/i.test(value)
  ? <details><summary>查看原始编号</summary><code>{value}</code></details>
  : <span>{value}</span>;
}
