import type { Pool, PoolConfig, PoolClient } from "pg";
import { IAMError, keyHash, type SessionStore } from "./iam-core";
import type { ProviderSessionBinding } from './iam-logout';

export function verifiedDatabaseOptions(raw: string, ca?: string): PoolConfig {
  const u = new URL(raw);
  // pg reparses connectionString and can override explicit ssl. Do not pass it.
  // No query parameters (including host/TLS overrides) or fragment are accepted.
  if (
    !["postgres:", "postgresql:"].includes(u.protocol) ||
    !u.hostname ||
    u.search ||
    u.hash ||
    !/^\/[^/]+$/.test(u.pathname) ||
    !u.username
  )
    throw new IAMError();
  return {
    host: u.hostname.replace(/^\[|\]$/g, ""),
    port: u.port ? Number(u.port) : 5432,
    user: decodeURIComponent(u.username),
    password: decodeURIComponent(u.password),
    database: decodeURIComponent(u.pathname.slice(1)),
    max: 5,
    connectionTimeoutMillis: 5000,
    query_timeout: 5000,
    ssl: { rejectUnauthorized: true, ...(ca ? { ca } : {}) },
  };
}

// Only ephemeral RP state is owned here. No reads/writes to identity or grant tables.
export class PostgresSessionStore implements SessionStore {
  constructor(private readonly pool: Pool) {}
  private table(namespace: "transaction" | "session") {
    return namespace === "transaction"
      ? "platform_browser_transaction"
      : "platform_browser_session";
  }
  async put(
    namespace: "transaction" | "session",
    key: string,
    value: Buffer,
    expiresAt: Date,
    provider?: ProviderSessionBinding,
  ) {
    const ttl = expiresAt.getTime() - Date.now();
    const maximum = namespace === "transaction" ? 300000 : 28800000;
    if (!Number.isFinite(ttl) || ttl <= 0 || ttl > maximum) throw new IAMError();
    if(provider) {
      if(namespace!=='session' || !provider.subject) throw new IAMError();
      return this.withProvider(provider,async (client, parent) => {
        if(parent.revoked || parent.subject_hash && !parent.subject_hash.equals(provider.subject!)) throw new IAMError();
        await client.query("UPDATE platform_browser_oidc_session SET subject_hash=$2,expires_at=GREATEST(expires_at,statement_timestamp()+interval '9 hours') WHERE scope_hash=$1",[provider.scope,provider.subject]);
        await client.query("INSERT INTO platform_browser_session(key_hash,encrypted_state,expires_at,oidc_session_hash) VALUES($1,$2,LEAST($3::timestamptz,statement_timestamp()+$4::double precision*interval '1 millisecond'),$5)",[keyHash(key),value,expiresAt,ttl,provider.scope]);
      });
    }
    // The database owns its TTL clock. Cap at both the original absolute
    // deadline and remaining lifetime; host/PG skew must never extend either.
    await this.pool.query(
      `INSERT INTO ${this.table(namespace)}(key_hash,encrypted_state,expires_at) VALUES($1,$2,LEAST($3::timestamptz,statement_timestamp()+$4::double precision*interval '1 millisecond'))`,
      [keyHash(key), value, expiresAt, ttl],
    );
  }
  async take(namespace: "transaction" | "session", key: string) {
    const result = await this.pool.query(
      `DELETE FROM ${this.table(namespace)} WHERE key_hash=$1 RETURNING encrypted_state,expires_at>statement_timestamp() AS active`,
      [keyHash(key)],
    );
    return result.rows[0]?.active
      ? (result.rows[0].encrypted_state as Buffer)
      : null;
  }
  async getSession(key: string) {
    const result = await this.pool.query(
      "SELECT encrypted_state FROM platform_browser_session WHERE key_hash=$1 AND expires_at>statement_timestamp()",
      [keyHash(key)],
    );
    return (result.rows[0]?.encrypted_state as Buffer | undefined) ?? null;
  }
  private async withProvider(binding: ProviderSessionBinding, action: (c: PoolClient,p:{subject_hash:Buffer|null;revoked:boolean})=>Promise<void>) {
    if(binding.scope.length!==32 || binding.subject && binding.subject.length!==32) throw new IAMError();
    const c=await this.pool.connect();
    try {
      await c.query('BEGIN');
      await c.query("INSERT INTO platform_browser_oidc_session(scope_hash,subject_hash,expires_at) VALUES($1,$2,statement_timestamp()+interval '9 hours') ON CONFLICT(scope_hash) DO NOTHING",[binding.scope,binding.subject??null]);
      const p=(await c.query('SELECT subject_hash,revoked FROM platform_browser_oidc_session WHERE scope_hash=$1 FOR UPDATE',[binding.scope])).rows[0];
      if(!p) throw new IAMError();
      await action(c,p); await c.query('COMMIT');
    } catch(e) { await c.query('ROLLBACK'); throw e; } finally { c.release(); }
  }
  async logoutProvider(binding: ProviderSessionBinding) {
    await this.withProvider(binding,async(c,p)=>{
      if(binding.subject && p.subject_hash && !p.subject_hash.equals(binding.subject)) return;
      await c.query("UPDATE platform_browser_oidc_session SET revoked=true,expires_at=GREATEST(expires_at,statement_timestamp()+interval '9 hours') WHERE scope_hash=$1",[binding.scope]);
      await c.query('DELETE FROM platform_browser_session WHERE oidc_session_hash=$1',[binding.scope]);
    });
  }
}
