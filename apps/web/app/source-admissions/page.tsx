import {displayText} from '../../lib/business-language.mjs';
import Link from "next/link";
import { headers } from "next/headers";
import { PageHeader } from "@/components/ui";
import { SourceAdmissionForm } from "@/components/source-admission-form";
import { admissionText } from "@/lib/source-admission-text";
import { sourceAdmissionConfig, sourceWebAuthorized } from "@/lib/source-admission-server";

export default async function SourceAdmissionsPage({ searchParams }: { searchParams: Promise<{ lang?: string }> }) {
  const lang = (await searchParams).lang === "en-US" ? "en-US" : "zh-CN";
  const t = admissionText[lang];
  const c = await sourceAdmissionConfig();
  return <section lang={lang} aria-label={t.title}>
    <PageHeader title={t.title} description={t.description} />
    <nav aria-label="Language"><Link href="/source-admissions?lang=zh-CN">中文</Link> · <Link href="/source-admissions?lang=en-US">English</Link></nav>
    {c && sourceWebAuthorized(await headers(), c) ? <SourceAdmissionForm lang={lang} scope={`${c.workspace}:${c.dataset}:${c.connection}:${c.principal}`} /> : <p role="status">{displayText(t.setup)}</p>}
  </section>;
}
