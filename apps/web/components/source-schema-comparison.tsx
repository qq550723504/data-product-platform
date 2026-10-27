import {displayText} from '../lib/business-language.mjs';
import type { SchemaColumn } from "@/lib/schema-observation";

export function SchemaColumnComparison({ baseline, columns, conclusive }: { baseline: SchemaColumn[]; columns: SchemaColumn[]; conclusive: boolean }) {
  return <div className="table-card" style={{ overflowX: "auto" }}><table className="data-table"><thead><tr><th>列</th><th>固定基线</th><th>本次扫描</th></tr></thead><tbody>
    {Array.from(new Set([...baseline.map(c => c.name), ...columns.map(c => c.name)])).map(name => {
      const b = baseline.find(c => c.name === name), c = columns.find(c => c.name === name);
      return <tr key={name}><td>{displayText(name)}</td><td>{displayText(b ? `${b.type} / ${b.constraint}` : "基线无此列")}</td><td>{displayText(c ? `${c.type} / ${c.constraint}` : conclusive ? "本次无此列" : "UNKNOWN")}</td></tr>;
    })}
  </tbody></table></div>;
}
