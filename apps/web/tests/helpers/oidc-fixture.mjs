import { createRequire } from "node:module";
import { readFileSync } from "node:fs";
import { createServer } from "node:https";
import { createHash } from "node:crypto";
import { generateKeyPair, exportJWK, SignJWT } from "jose";
const require = createRequire(import.meta.url);
const { BrowserIAM, randomKey } = require("../../.iam-tests/iam-core.js");
export class MemoryStore {
  values = new Map();
  providers = new Map();
  async put(ns, key, value, expiry, binding) {
    const scope=binding?.scope.toString('hex'), subject=binding?.subject?.toString('hex');
    if(scope) {
      const p=this.providers.get(scope);
      if(p?.revoked || p?.subject && p.subject!==subject) throw new Error('Provider session revoked');
      this.providers.set(scope,{subject,revoked:false});
    }
    this.values.set(`${ns}:${key}`, { value, expiry, scope });
  }
  async take(ns, key) {
    const name = `${ns}:${key}`,
      row = this.values.get(name);
    this.values.delete(name);
    return row && row.expiry > Date.now() ? row.value : null;
  }
  async getSession(key) {
    const row = this.values.get(`session:${key}`);
    return row && row.expiry > Date.now() ? row.value : null;
  }
  async logoutProvider(binding) {
    const scope=binding.scope.toString('hex'),subject=binding.subject?.toString('hex'),p=this.providers.get(scope);
    if(subject && p?.subject && subject!==p.subject) return;
    this.providers.set(scope,{subject:p?.subject??subject,revoked:true});
    for(const [key,row] of this.values) if(row.scope===scope) this.values.delete(key);
  }
}
export async function fixture(t, store = new MemoryStore()) {
  const { publicKey, privateKey } = await generateKeyPair("RS256");
  const jwk = await exportJWK(publicKey);
  jwk.kid = "fixture";
  const codes = new Map();
  const state = {
    revision: "1:1:1",
    deny: false,
    principalCalls: [],
    tokenCalls: 0,
  };
  const people = {
    one: "11111111-1111-4111-8111-111111111111",
    two: "22222222-2222-4222-8222-222222222222",
  };
  let cfg;
  const server = createServer(
    {
      cert: readFileSync(process.env.IAM_FIXTURE_CERT),
      key: readFileSync(process.env.IAM_FIXTURE_KEY),
    },
    async (req, res) => {
      res.setHeader("Content-Type", "application/json");
      if (req.url === "/jwks") {
        res.end(JSON.stringify({ keys: [jwk] }));
        return;
      }
      if (req.url === "/token") {
        state.tokenCalls++;
        let body = "";
        for await (const chunk of req) body += chunk;
        const params = new URLSearchParams(body),
          code = codes.get(params.get("code"));
        codes.delete(params.get("code"));
        if (
          !code ||
          Buffer.from(req.headers.authorization?.slice(6) ?? "", "base64")
            .toString()
            .split(":")
            .map(decodeURIComponent)
            .join(":") !== "rp:fixture-secret" ||
          params.get("redirect_uri") !== `${cfg.origin}/auth/callback` ||
          createHash("sha256")
            .update(params.get("code_verifier") ?? "")
            .digest("base64url") !== code.challenge
        ) {
          res.writeHead(400);
          res.end("{}");
          return;
        }
        const now = Math.floor(Date.now() / 1000);
        const jwt = await new SignJWT({
          iss: cfg.issuer,
          sub: code.subject,
          aud: "rp",
          iat: now,
          exp: now + 120,
          nonce: code.nonce,
          ...code.claims,
        })
          .setProtectedHeader({ alg: "RS256", kid: "fixture" })
          .sign(code.wrongKey ?? privateKey);
        res.end(
          JSON.stringify({
            access_token: `access-${code.subject}`,
            id_token: jwt,
            token_type: "Bearer",
            expires_in: 120,
            ...code.token,
          }),
        );
        return;
      }
      if (req.url === "/internal/identity/business-acceptance-principal") {
        state.principalCalls.push(req.headers.authorization);
        let raw = "";
        for await (const chunk of req) raw += chunk;
        if (
          state.deny ||
          JSON.parse(raw).workspace_id !== cfg.workspaceId ||
          !["Bearer access-one", "Bearer access-two"].includes(
            req.headers.authorization,
          )
        ) {
          res.writeHead(403);
          res.end("{}");
          return;
        }
        const subject = req.headers.authorization.endsWith("one")
          ? "one"
          : "two";
        res.end(
          JSON.stringify({
            profileVersion:"business-acceptance.v1",
            principalId: people[subject],
            canonicalSubject:people[subject],
            workspaceId: cfg.workspaceId,
            issuer: cfg.issuer,
            subject,
            authorityRevision: state.revision,
            capabilities: ["BUSINESS_ACCEPTANCE_READ"],
            expiresAt: new Date(Date.now() + 60000).toISOString(),
            ...state.principalOverride,
          }),
        );
        return;
      }
      if (state.businessHandler) { await state.businessHandler(req, res); return; }
      res.writeHead(404);
      res.end("{}");
    },
  );
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(() => new Promise((resolve) => server.close(resolve)));
  const base = `https://127.0.0.1:${server.address().port}`;
  cfg = {
    profile:"business-acceptance",
    origin: "https://console.example.test",
    issuer: "https://identity.example.test",
    clientId: "rp",
    clientSecret: "fixture-secret",
    authorizationEndpoint: `${base}/authorize`,
    tokenEndpoint: `${base}/token`,
    jwksURI: `${base}/jwks`,
    logoutEndpoint: `${base}/logout`,
    principalEndpoint: `${base}/internal/identity/business-acceptance-principal`,
    workspaceId: "33333333-3333-4333-8333-333333333333",
    scope: "openid business-acceptance",
    encryptionKey: Buffer.alloc(32, 7),
  };
  const iam = new BrowserIAM(cfg, store);
  const callback = (started, subject = "one", extra = {}) => {
    const code = randomKey();
    codes.set(code, {
      subject,
      nonce: started.url.searchParams.get("nonce"),
      challenge: started.url.searchParams.get("code_challenge"),
      ...extra,
    });
    return new URL(
      `${cfg.origin}/auth/callback?code=${code}&state=${started.url.searchParams.get("state")}`,
    );
  };
  const logoutToken = async (sid, claims={}, key=privateKey) => {
    const now=Math.floor(Date.now()/1000);
    const p={iss:cfg.issuer,aud:cfg.clientId,sub:'one',sid,iat:now,exp:now+120,jti:randomKey(),events:{'http://schemas.openid.net/event/backchannel-logout':{}},...claims};
    for(const k of Object.keys(p)) if(p[k]===undefined) delete p[k];
    return new SignJWT(p).setProtectedHeader({alg:'RS256',kid:'fixture',typ:'logout+jwt'}).sign(key);
  };
  return { iam, cfg, state, callback, store, logoutToken };
}
