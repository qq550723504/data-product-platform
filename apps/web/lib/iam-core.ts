import {
  createCipheriv,
  createDecipheriv,
  createHash,
  randomBytes,
  timingSafeEqual,
} from "node:crypto";
import * as oidc from "openid-client";
import type { IAMProfile } from "./iam-mode";
import { logoutValidator, providerSession, type ProviderSessionBinding } from './iam-logout';

export const cookieNames = {
  csrf: "__Host-dpp-login-csrf",
  handshake: "__Host-dpp-oidc",
  session: "__Host-dpp-session",
};
export const cookieOptions = {
  secure: true,
  httpOnly: true,
  sameSite: "lax" as const,
  path: "/",
};
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const opaque = /^[A-Za-z0-9_-]{43}$/;
export class IAMError extends Error {
  constructor() {
    super("Browser identity unavailable");
  }
}
export function randomKey() {
  return randomBytes(32).toString("base64url");
}
export function keyHash(key: string): Buffer {
  if (!opaque.test(key)) throw new IAMError();
  return createHash("sha256").update(key).digest();
}
export function equal(a: string, b: string) {
  const x = Buffer.from(a),
    y = Buffer.from(b);
  return x.length > 0 && x.length === y.length && timingSafeEqual(x, y);
}
export function httpsURL(raw: string): URL {
  const u = new URL(raw);
  if (
    u.protocol !== "https:" ||
    !u.hostname ||
    u.username ||
    u.password ||
    u.hash ||
    u.search
  )
    throw new IAMError();
  return u;
}
// Chromium applies form-action to redirects after a same-origin form POST.
// Admit only the configured HTTPS protocol endpoint origin; never a request URL.
export function oidcFormPolicy(endpoint?: string): string {
  const provider = endpoint ? ` ${httpsURL(endpoint).origin}` : "";
  return `default-src 'none'; form-action 'self'${provider}; frame-ancestors 'none'`;
}
export type IAMConfig = {
  profile?: IAMProfile;
  annotationWorkspaceId?: string;
  origin: string;
  issuer: string;
  clientId: string;
  clientSecret: string;
  authorizationEndpoint: string;
  tokenEndpoint: string;
  jwksURI: string;
  logoutEndpoint?: string;
  backchannelLogoutEnabled?: boolean;
  principalEndpoint: string;
  workspaceId: string;
  scope: string;
  encryptionKey: Buffer;
};
export function validateConfig(c: IAMConfig): IAMConfig {
  if (c.backchannelLogoutEnabled !== undefined && typeof c.backchannelLogoutEnabled !== 'boolean' || c.backchannelLogoutEnabled && !c.logoutEndpoint) throw new IAMError();
  if (process.env.NODE_TLS_REJECT_UNAUTHORIZED === "0") throw new IAMError();
  if (
    httpsURL(c.origin).origin !== c.origin ||
    !uuid.test(c.workspaceId) ||
    c.workspaceId === "00000000-0000-0000-0000-000000000000" ||
    !c.clientId ||
    !c.clientSecret ||
    c.encryptionKey.length !== 32 ||
    !c.scope.split(" ").includes("openid") ||
    c.scope.split(" ").includes("offline_access")
  )
    throw new IAMError();
  for (const u of [
    c.issuer,
    c.authorizationEndpoint,
    c.tokenEndpoint,
    c.jwksURI,
    c.principalEndpoint,
    ...(c.logoutEndpoint ? [c.logoutEndpoint] : []),
  ])
    httpsURL(u);
  const profile=c.profile;
  if(!profile||!["business-acceptance","platform-management"].includes(profile)||c.backchannelLogoutEnabled||c.annotationWorkspaceId||new URL(c.principalEndpoint).pathname!==`/internal/identity/${profile}-principal`||!c.scope.split(" ").includes(profile)||c.scope.split(" ").includes(profile==="business-acceptance"?"platform-management":"business-acceptance"))throw new IAMError();
  return c;
}
export function checkOrigin(c: IAMConfig, url: URL, origin?: string | null) {
  if (url.origin !== c.origin || (origin !== undefined && origin !== c.origin))
    throw new IAMError();
}
export function checkBrowserRequest(c: IAMConfig, r: Request, post = false) {
  checkOrigin(c, new URL(r.url), post ? r.headers.get("origin") : undefined);
  if (
    r.headers.get("host") !== new URL(c.origin).host ||
    r.headers.has("authorization")
  )
    throw new IAMError();
}
export async function readCSRFForm(request: Request): Promise<string> {
  if (
    request.headers.get("content-type")?.split(";")[0].trim().toLowerCase() !==
    "application/x-www-form-urlencoded"
  )
    throw new IAMError();
  const length = request.headers.get("content-length");
  if (length !== null && (!/^[0-9]+$/.test(length) || Number(length) > 4096))
    throw new IAMError();
  const reader = request.body?.getReader();
  if (!reader) throw new IAMError();
  let timer: ReturnType<typeof setTimeout> | undefined;
  const deadline = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new IAMError()), 10000);
  });
  const chunks: Uint8Array[] = [];
  let bytes = 0;
  try {
    for (;;) {
      const { done, value } = await Promise.race([reader.read(), deadline]);
      if (done) break;
      bytes += value.length;
      if (bytes > 4096) throw new IAMError();
      chunks.push(value);
    }
    const form = new URLSearchParams(
      new TextDecoder("utf-8", { fatal: true }).decode(Buffer.concat(chunks)),
    );
    if (
      form.getAll("csrf").length !== 1 ||
      [...form.keys()].some((key) => key !== "csrf")
    )
      throw new IAMError();
    return form.get("csrf") ?? "";
  } finally {
    clearTimeout(timer);
    await reader.cancel();
  }
}
export function seal(key: Buffer, namespace: string, value: unknown): Buffer {
  const iv = randomBytes(12),
    cipher = createCipheriv("aes-256-gcm", key, iv);
  cipher.setAAD(Buffer.from(namespace));
  const encrypted = Buffer.concat([
    cipher.update(JSON.stringify(value)),
    cipher.final(),
  ]);
  return Buffer.concat([iv, cipher.getAuthTag(), encrypted]);
}
export function open<T>(key: Buffer, namespace: string, value: Buffer): T {
  try {
    const decipher = createDecipheriv(
      "aes-256-gcm",
      key,
      value.subarray(0, 12),
    );
    decipher.setAAD(Buffer.from(namespace));
    decipher.setAuthTag(value.subarray(12, 28));
    return JSON.parse(
      Buffer.concat([
        decipher.update(value.subarray(28)),
        decipher.final(),
      ]).toString(),
    ) as T;
  } catch {
    throw new IAMError();
  }
}
export type Principal = {
  canonical_subject: string;
  principal_id: string;
  workspace_id: string;
  issuer: string;
  subject: string;
  revision: string;
  capabilities: string[];
  expires_at: string;
};
type Transaction = { state: string; nonce: string; verifier: string; admission: string };
type Session = {
  admission: string;
  accessToken: string;
  issuer: string;
  subject: string;
  principal: Principal;
  csrf: string;
  expiresAt: number;
  provider?: {scope: string; subject: string};
};
export interface SessionStore {
  put(
    namespace: "transaction" | "session",
    key: string,
    value: Buffer,
    expiresAt: Date,
    provider?: ProviderSessionBinding,
  ): Promise<void>;
  take(
    namespace: "transaction" | "session",
    key: string,
  ): Promise<Buffer | null>;
  getSession(key: string): Promise<Buffer | null>;
  logoutProvider(binding: ProviderSessionBinding): Promise<void>;
}

// HTTPS transport is shared by mature RP protocol and identity confirmation.
// No global TLS overrides, redirects, unlimited bodies, or raw upstream errors.
export function boundedFetch(fetcher: typeof fetch = fetch, options: {allowErrorResponse?: boolean; allowEmptyErrorResponse?: boolean; maxBytes?: number} = {}): typeof fetch {
  return async (input, init) => {
    const response = await fetcher(input, {
      ...init,
      redirect: "manual",
      signal: AbortSignal.timeout(10000),
      cache: "no-store",
    });
    if (
      (!response.ok && !options.allowErrorResponse) ||
      (response.status >= 300 && response.status < 400) ||
      response.headers.get("content-type")?.split(";")[0].trim() !==
        "application/json"
    ) {
      await response.body?.cancel();
      throw new IAMError();
    }
    const reader = response.body?.getReader();
    if (!reader) throw new IAMError();
    const chunks: Uint8Array[] = [];
    let size = 0;
    try {
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        size += value.length;
        if (size > (options.maxBytes ?? 65536)) throw new IAMError();
        chunks.push(value);
      }
    } finally {
      await reader.cancel();
    }
    const body = Buffer.concat(chunks);
    try {
      if (!(body.length === 0 && !response.ok && options.allowEmptyErrorResponse)) JSON.parse(body.toString());
    } catch {
      throw new IAMError();
    }
    return new Response(body, {
      status: response.status,
      headers: response.headers,
    });
  };
}

export class BrowserIAM {
  private readonly rp: oidc.Configuration;
  private readonly request: typeof fetch;
  private readonly reviewTransport: typeof fetch;
  private readonly annotationTransport: typeof fetch;
  private readonly admission: string;
  private readonly verifyLogout: ReturnType<typeof logoutValidator>;
  constructor(
    readonly config: IAMConfig,
    private readonly store: SessionStore,
    fetcher: typeof fetch = fetch,
  ) {
    validateConfig(config);
    this.admission = JSON.stringify([config.profile,config.origin,config.issuer,config.clientId,config.scope,config.authorizationEndpoint,config.tokenEndpoint,config.jwksURI,config.logoutEndpoint??"",config.principalEndpoint,config.workspaceId,!!config.backchannelLogoutEnabled]);
    this.request = boundedFetch(fetcher);
    this.verifyLogout = logoutValidator(config,this.request);
    this.reviewTransport = boundedFetch(fetcher, {allowErrorResponse:true, maxBytes:1048576});
    this.annotationTransport = boundedFetch(fetcher, {allowErrorResponse:true, allowEmptyErrorResponse:true, maxBytes:4096});
    this.rp = new oidc.Configuration(
      {
        issuer: config.issuer,
        authorization_endpoint: config.authorizationEndpoint,
        token_endpoint: config.tokenEndpoint,
        jwks_uri: config.jwksURI,
        ...(config.logoutEndpoint
          ? { end_session_endpoint: config.logoutEndpoint }
          : {}),
      },
      config.clientId,
      {
        client_secret: config.clientSecret,
        id_token_signed_response_alg: "RS256",
      },
      oidc.ClientSecretBasic(config.clientSecret),
    );
    this.rp.timeout = 10;
    this.rp[oidc.customFetch] = this.request;
    oidc.enableNonRepudiationChecks(this.rp); // Mandatory ID-token signature/JWKS checks, also for TLS token endpoint responses.
  }
  async start() {
    const transaction: Transaction = {
      admission: this.admission,
      state: oidc.randomState(),
      nonce: oidc.randomNonce(),
      verifier: oidc.randomPKCECodeVerifier(),
    };
    const key = randomKey();
    await this.store.put(
      "transaction",
      key,
      seal(this.config.encryptionKey, "transaction", transaction),
      new Date(Date.now() + 300000),
    );
    const url = oidc.buildAuthorizationUrl(this.rp, {
      redirect_uri: `${this.config.origin}/auth/callback`,
      response_type: "code",
      scope: this.config.scope,
      state: transaction.state,
      nonce: transaction.nonce,
      code_challenge: await oidc.calculatePKCECodeChallenge(
        transaction.verifier,
      ),
      code_challenge_method: "S256",
    });
    return { key, url };
  }
  async callback(url: URL, browserKey: string, previousSession?: string) {
    checkOrigin(this.config, url);
    if (url.pathname !== "/auth/callback") throw new IAMError();
    // Lookup by the HttpOnly browser handshake cookie, NEVER by query.state.
    const raw = await this.store.take("transaction", browserKey);
    if (!raw) throw new IAMError();
    try {
      const t = open<Transaction>(
        this.config.encryptionKey,
        "transaction",
        raw,
      );
      if (t.admission !== this.admission) throw new IAMError();
      const tokens = await oidc.authorizationCodeGrant(this.rp, url, {
        pkceCodeVerifier: t.verifier,
        expectedState: t.state,
        expectedNonce: t.nonce,
        idTokenExpected: true,
      });
      const claims = tokens.claims(),
        life = tokens.expiresIn();
      if (
        !claims ||
        typeof claims.sub !== "string" ||
        !claims.sub ||
        claims.iss !== this.config.issuer ||
        !Number.isFinite(life) ||
        !life ||
        life <= 0 ||
        tokens.token_type.toLowerCase() !== "bearer" ||
        typeof tokens.access_token !== "string" ||
        tokens.access_token.length > 8192
      )
        throw new IAMError();
      const principal = await this.confirm(
        tokens.access_token,
        claims.iss,
        claims.sub,
      );
      const expiresAt = Math.min(
        Date.now() + life * 1000,
        claims.exp * 1000,
        Date.now() + 8 * 3600000,
      );
      const provider = this.config.backchannelLogoutEnabled ? providerSession(this.config,claims.sid,claims.sub) : undefined;
      const key = randomKey(),
        session: Session = {
          admission: this.admission,
          accessToken: tokens.access_token,
          issuer: claims.iss,
          subject: claims.sub,
          principal,
          csrf: randomKey(),
          expiresAt,
          ...(provider ? {provider:{scope:provider.scope.toString('hex'),subject:provider.subject!.toString('hex')}} : {}),
        };
      if (previousSession) await this.store.take("session", previousSession);
      await this.store.put(
        "session",
        key,
        seal(this.config.encryptionKey, "session", session),
        new Date(expiresAt),
        provider,
      );
      return { key, expiresAt };
    } catch {
      throw new IAMError();
    }
  }
  private async confirm(
    token: string,
    issuer: string,
    subject: string,
  ): Promise<Principal> {
    const response = await this.request(this.config.principalEndpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ workspace_id: this.config.workspaceId }),
    });
    const wire = await response.json();
    const management=this.config.profile==="platform-management";
    const allowedCaps=management?["PLATFORM_MANAGEMENT_READ"]:["BUSINESS_ACCEPTANCE_READ","BUSINESS_ACCEPTANCE_DRAFT","BUSINESS_ACCEPTANCE_BASIS_CONFIRM","BUSINESS_RECEIPT_RECORD"];
    if(!wire || Object.keys(wire).sort().join(",")!=="authorityRevision,canonicalSubject,capabilities,expiresAt,issuer,principalId,profileVersion,subject,workspaceId" || wire.profileVersion!==`${this.config.profile}.v1`) throw new IAMError();
    const p:Principal={canonical_subject:wire.canonicalSubject,principal_id:wire.principalId,workspace_id:wire.workspaceId,issuer:wire.issuer,subject:wire.subject,revision:wire.authorityRevision,capabilities:wire.capabilities,expires_at:wire.expiresAt};
    if(!uuid.test(p.principal_id) || p.principal_id==="00000000-0000-0000-0000-000000000000" || p.canonical_subject!==p.principal_id || p.workspace_id!==this.config.workspaceId || p.issuer!==issuer || p.subject!==subject || !/^[1-9][0-9]*:[1-9][0-9]*:[1-9][0-9]*$/.test(p.revision) || !Array.isArray(p.capabilities) || !p.capabilities.includes(allowedCaps[0]) || new Set(p.capabilities).size!==p.capabilities.length || p.capabilities.some(c=>!allowedCaps.includes(c)) || !Number.isFinite(Date.parse(p.expires_at)) || Date.parse(p.expires_at)<=Date.now() || Date.parse(p.expires_at)>Date.now()+300000) throw new IAMError();
    return p;
  }

  private async authorizedSession(key: string) {
    try {
      const raw = await this.store.getSession(key);
      if (!raw) throw new IAMError();
      const session = open<Session>(this.config.encryptionKey, "session", raw);
      if (session.expiresAt <= Date.now() || session.admission !== this.admission) throw new IAMError();
      const principal = await this.confirm(
        session.accessToken,
        session.issuer,
        session.subject,
      );
      if (
        principal.principal_id !== session.principal.principal_id || principal.canonical_subject !== session.principal.canonical_subject ||
        principal.revision !== session.principal.revision
      )
        throw new IAMError();
      return { principal, session };
    } catch {
      throw new IAMError();
    }
  }
  async current(key: string) {
    const {principal, session} = await this.authorizedSession(key);
    return {principal, csrf:session.csrf}; // Never export credentials to React/API callers.
  }
  async acceptanceClient(key: string) {
    if(this.config.profile!=="business-acceptance")throw new IAMError();
    const {principal} = await this.authorizedSession(key);
    const apiBaseUrl=new URL(this.config.principalEndpoint).origin;
    const request:typeof fetch=async(input,init={})=>{
      const url=new URL(typeof input==="string"?input:input instanceof URL?input.href:input.url);
      const method=init.method??"GET", h=new Headers(init.headers);
      const prefix=`/api/v1/workspaces/${this.config.workspaceId}/`;
      const allowed=url.pathname===prefix+"acceptance-context" || url.pathname===prefix+"contract-versions" || url.pathname===prefix+"acceptance-targets" || url.pathname===prefix+"acceptance-basis" || new RegExp("^"+prefix+"acceptance-materials/[0-9a-f-]{36}$").test(url.pathname) || /^\/api\/v1\/contract-versions\/[0-9a-f-]{36}$/.test(url.pathname) && url.searchParams.get("workspaceId")===this.config.workspaceId;
      if(url.origin!==apiBaseUrl || url.username || url.password || url.hash || method!=="GET" || init.body!==undefined || [...h.keys()].length || !allowed) throw new IAMError();
      const {session}=await this.authorizedSession(key);
      h.set("Authorization",`Bearer ${session.accessToken}`);
      return this.reviewTransport(url,{...init,headers:h});
    };
    return {principal,apiBaseUrl,request};
  }
  async managementClient(key:string){
    if(this.config.profile!=="platform-management")throw new IAMError();
    const {principal}=await this.authorizedSession(key),apiBaseUrl=new URL(this.config.principalEndpoint).origin;
    const request:typeof fetch=async(input,init={})=>{
      const url=new URL(typeof input==="string"?input:input instanceof URL?input.href:input.url);
      const prefix=`/api/v1/workspaces/${this.config.workspaceId}/management/`;
      if(url.origin!==apiBaseUrl||url.username||url.password||url.hash||(init.method??"GET")!=="GET"||init.body!==undefined||[...new Headers(init.headers).keys()].length||!["members","roles","audit","users"].some(resource=>url.pathname===prefix+resource))throw new IAMError();
      const {session}=await this.authorizedSession(key);
      const response=await this.reviewTransport(url,{...init,method:"GET",cache:"no-store",headers:{Authorization:`Bearer ${session.accessToken}`}});
      await this.authorizedSession(key);
      return response;
    };
    return {principal,apiBaseUrl,request};
  }
  async logout(key: string, csrf: string) {
    // CSRF check does not depend on a working IdP. Invalid/revoked sessions can still be contained.
    const raw = await this.store.getSession(key);
    if (!raw) throw new IAMError();
    const session = open<Session>(this.config.encryptionKey, "session", raw);
    if (!equal(csrf, session.csrf)) throw new IAMError();
    if (!(await this.store.take("session", key))) throw new IAMError();
    if(this.config.backchannelLogoutEnabled) {
      const p=session.provider;
      if(!p || !/^[a-f0-9]{64}$/.test(p.scope) || !/^[a-f0-9]{64}$/.test(p.subject)) throw new IAMError();
      await this.store.logoutProvider({scope:Buffer.from(p.scope,'hex'),subject:Buffer.from(p.subject,'hex')});
    }
    return this.config.logoutEndpoint
      ? oidc.buildEndSessionUrl(this.rp, {
          client_id: this.config.clientId,
          post_logout_redirect_uri: `${this.config.origin}/auth/logged-out`,
        })
      : new URL(`${this.config.origin}/auth/logged-out`);
  }
  async logoutForm(key: string) {
    const raw = await this.store.getSession(key);
    if (!raw) throw new IAMError();
    return open<Session>(this.config.encryptionKey, "session", raw).csrf;
  }
  async backchannelLogout(token: string) {
    try {
      if(!this.config.backchannelLogoutEnabled) throw new IAMError();
      await this.store.logoutProvider(await this.verifyLogout(token));
    } catch { throw new IAMError(); }
  }
}
