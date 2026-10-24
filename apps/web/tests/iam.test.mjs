import { test } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFileSync } from "node:fs";

import { randomUUID } from "node:crypto";
import { generateKeyPair } from "jose";
import pg from "pg";
const require = createRequire(import.meta.url);
const {
  BrowserIAM,
  seal,
  open,
  randomKey,
  checkOrigin,
  validateConfig,
  cookieOptions,
  cookieNames,
  boundedFetch,
} = require("../.iam-tests/iam-core.js");
const { PostgresSessionStore,verifiedDatabaseOptions } = require("../.iam-tests/iam-store.js");

import { fixture, MemoryStore } from "./helpers/oidc-fixture.mjs";
async function pgStore(t) {
  const raw = process.env.IAM_TEST_POSTGRES_DSN;
  if (!raw) return new MemoryStore();
  const u = new URL(raw);
  assert.equal(u.hostname, "127.0.0.1");
  assert.equal(u.pathname, "/acceptance_1a_unit");
  const databaseOptions=verifiedDatabaseOptions(raw,readFileSync(process.env.IAM_TEST_POSTGRES_CA,'utf8'));
  const admin = new pg.Pool(databaseOptions),
    schema = `browser_iam_${randomUUID().replaceAll("-", "")}`;
  await admin.query(`CREATE SCHEMA ${schema}`);
  const pool = new pg.Pool({
    ...databaseOptions,
    options: `-csearch_path=${schema}`,
  });
  await pool.query(
    readFileSync(
      "../../migrations/000076_acceptance_browser_session.up.sql",
      "utf8",
    ),
  );
  t.after(async () => {
    await pool.end();
    await admin.query(`DROP SCHEMA ${schema} CASCADE`);
    await admin.end();
  });
  return new PostgresSessionStore(pool);
}
test("real SDK TLS code/PKCE/JWKS and two separate canonical people; credentials never exported", async (t) => {
  const f = await fixture(t, await pgStore(t));
  const first = await f.iam.start();
  assert.equal(first.url.searchParams.get("code_challenge_method"), "S256");
  const one = await f.iam.callback(f.callback(first), first.key);
  const second = await f.iam.start(),
    two = await f.iam.callback(f.callback(second, "two"), second.key);
  const current = await f.iam.current(one.key),
    other = await f.iam.current(two.key);
  assert.notEqual(current.principal.principal_id, other.principal.principal_id);
  assert.equal(JSON.stringify(current).includes("access-"), false);
  assert.equal(JSON.stringify(one).includes("token"), false);
  assert.deepEqual(f.state.principalCalls, [
    "Bearer access-one",
    "Bearer access-two",
    "Bearer access-one",
    "Bearer access-two",
  ]);
});
test("browser-bound handshake rejects stolen state/code across browsers, missing cookie, mismatched state and callback replay", async (t) => {
  const f = await fixture(t, await pgStore(t)),
    a = await f.iam.start(),
    b = await f.iam.start(),
    url = f.callback(a);
  await assert.rejects(f.iam.callback(url, b.key));
  assert.equal(f.state.tokenCalls, 0);
  await assert.rejects(f.iam.callback(url, randomKey()));
  assert.equal(f.state.tokenCalls, 0);
  await f.iam.callback(url, a.key);
  await assert.rejects(f.iam.callback(url, a.key));
  assert.equal(f.state.tokenCalls, 1);
});
test("parallel replicas consume one callback winner with real PG atomic state when configured", async (t) => {
  const f = await fixture(t, await pgStore(t)),
    replica = new BrowserIAM(f.cfg, f.store),
    a = await f.iam.start(),
    url = f.callback(a);
  const results = await Promise.allSettled([
    f.iam.callback(url, a.key),
    replica.callback(url, a.key),
  ]);
  assert.equal(results.filter((r) => r.status === "fulfilled").length, 1);
  assert.equal(f.state.tokenCalls, 1);
});
test("provider denial consumes handshake once and wrong nonce/signature fails mature verification", async (t) => {
  const f = await fixture(t);
  const a = await f.iam.start(),
    denial = new URL(
      `${f.cfg.origin}/auth/callback?error=access_denied&state=${a.url.searchParams.get("state")}`,
    );
  await assert.rejects(f.iam.callback(denial, a.key));
  await assert.rejects(f.iam.callback(f.callback(a), a.key));
  assert.equal(f.state.tokenCalls, 0);
  const b = await f.iam.start();
  await assert.rejects(
    f.iam.callback(f.callback(b, "one", { nonce: "wrong" }), b.key),
  );
  const { privateKey } = await generateKeyPair("RS256"),
    c = await f.iam.start();
  await assert.rejects(
    f.iam.callback(f.callback(c, "one", { wrongKey: privateKey }), c.key),
  );
  assert.equal(f.state.principalCalls.length, 0);
});
test("mature RP rejects wrong issuer/audience/expired ID token and missing ID token before authority lookup", async (t) => {
  const f = await fixture(t);
  for (const extra of [
    { claims: { iss: "https://evil.example.test" } },
    { claims: { aud: "other-client" } },
    { claims: { exp: 1 } },
    { token: { id_token: undefined } },
  ]) {
    const started = await f.iam.start();
    await assert.rejects(
      f.iam.callback(f.callback(started, "one", extra), started.key),
    );
  }
  assert.equal(f.state.principalCalls.length, 0);
});
test("absolute credential lifetime requires new login; no refresh or static-token fallback", async (t) => {
  const f = await fixture(t, await pgStore(t)),
    started = await f.iam.start();
  const session = await f.iam.callback(
    f.callback(started, "one", {
      token: { expires_in: 1, refresh_token: "must-not-persist" },
    }),
    started.key,
  );
  await new Promise((resolve) => setTimeout(resolve, 1100));
  await assert.rejects(f.iam.current(session.key));
  assert.equal(f.state.principalCalls.length, 1);
});
test("current workspace grant/actor/binding revision and provider errors deny old sessions; re-enable cannot revive", async (t) => {
  const f = await fixture(t),
    a = await f.iam.start(),
    s = await f.iam.callback(f.callback(a), a.key);
  f.state.deny = true;
  await assert.rejects(f.iam.current(s.key));
  f.state.deny = false;
  f.state.revision = "1:1:3";
  await assert.rejects(f.iam.current(s.key));
  f.state.revision = "1:1:1";
  f.state.principalOverride = {
    workspaceId: "44444444-4444-4444-8444-444444444444",
  };
  await assert.rejects(f.iam.current(s.key));
  f.state.principalOverride = {
    principalId: "22222222-2222-4222-8222-222222222222",
  };
  await assert.rejects(f.iam.current(s.key));
  f.state.principalOverride = { subject: "two" };
  await assert.rejects(f.iam.current(s.key));
});
test("local logout works during authority outage, requires CSRF and rejects replay; redirects fixed without token hints", async (t) => {
  const f = await fixture(t, await pgStore(t)),
    a = await f.iam.start(),
    s = await f.iam.callback(f.callback(a), a.key);
  const view = await f.iam.current(s.key);
  await assert.rejects(f.iam.logout(s.key, randomKey()));
  f.state.deny = true;
  assert.equal(await f.iam.logoutForm(s.key), view.csrf);
  const url = await f.iam.logout(s.key, view.csrf);
  assert.equal(
    url.searchParams.get("post_logout_redirect_uri"),
    `${f.cfg.origin}/auth/logged-out`,
  );
  assert.equal(url.searchParams.has("id_token_hint"), false);
  await assert.rejects(f.iam.current(s.key));
  await assert.rejects(f.iam.logout(s.key, view.csrf));
});
test("canonical origin/TLS config, AES-GCM purpose isolation, opaque cookies and bounded provider transport", async () => {
  const key = Buffer.alloc(32, 7),
    blob = seal(key, "session", { secret: "token" });
  assert.equal(blob.includes(Buffer.from("token")), false);
  assert.equal(open(key, "session", blob).secret, "token");
  assert.throws(() => open(key, "transaction", blob));
  assert.throws(() => open(Buffer.alloc(32, 8), "session", blob));
  blob[31] ^= 1;
  assert.throws(() => open(key, "session", blob));
  assert.deepEqual(cookieOptions, {
    secure: true,
    httpOnly: true,
    sameSite: "lax",
    path: "/",
  });
  assert.equal(
    Object.values(cookieNames).every((k) => k.startsWith("__Host-")),
    true,
  );
  assert.throws(() =>
    checkOrigin(
      { origin: "https://console.example.test" },
      new URL("https://evil.example.test/auth/callback"),
    ),
  );
  assert.throws(() =>
    checkOrigin(
      { origin: "https://console.example.test" },
      new URL("https://console.example.test/auth/login"),
      "https://evil.example.test",
    ),
  );
  assert.throws(() =>
    validateConfig({
      origin: "http://localhost",
      workspaceId: "x",
      encryptionKey: key,
    }),
  );
  for (const response of [
    new Response("{}", {
      status: 302,
      headers: {
        "content-type": "application/json",
        location: "https://evil.example.test",
      },
    }),
    new Response("x".repeat(65537), {
      headers: { "content-type": "application/json" },
    }),
    new Response("{}{}", { headers: { "content-type": "application/json" } }),
  ]) {
    await assert.rejects(
      boundedFetch(async () => response)("https://fixture.example.test"),
    );
  }
});
test(
  "real PG expires state, requires bounded deadlines and drain before down",
  { skip: !process.env.IAM_TEST_POSTGRES_DSN },
  async (t) => {
    const raw = process.env.IAM_TEST_POSTGRES_DSN,
      pool = new pg.Pool({ connectionString: raw }),
      schema = `browser_iam_${randomUUID().replaceAll("-", "")}`;
    await pool.query(`CREATE SCHEMA ${schema}`);
    const db = new pg.Pool({
      connectionString: raw,
      options: `-csearch_path=${schema}`,
    });
    t.after(async () => {
      await db.end();
      await pool.query(`DROP SCHEMA ${schema} CASCADE`);
      await pool.end();
    });
    await db.query(
      readFileSync(
        "../../migrations/000076_acceptance_browser_session.up.sql",
        "utf8",
      ),
    );
    const store = new PostgresSessionStore(db),
      key = randomKey(),
      cipher = seal(Buffer.alloc(32, 1), "transaction", { state: "test" });
    const realNow = Date.now;
    try {
      for (const namespace of ["transaction", "session"]) {
        for (const skew of [-60000,60000]) {
          Date.now = () => realNow()+skew;
          const lifetime = namespace === "transaction" ? 300000 : 28800000;
          const expiry = new Date(Date.now()+lifetime), skewKey = randomKey();
          await store.put(namespace,skewKey,cipher,expiry);
          const table = namespace === "transaction" ? "platform_browser_transaction" : "platform_browser_session";
          const {rows} = await db.query(`SELECT expires_at<=created_at+$1::double precision*interval '1 millisecond' AS within_policy,expires_at<=$2::timestamptz AS not_extended FROM ${table}`, [lifetime,expiry]);
          assert.equal(rows.length,1);assert.equal(rows[0].within_policy,true);assert.equal(rows[0].not_extended,true);
          await store.take(namespace,skewKey);
        }
      }
    } finally { Date.now = realNow; }
    await store.put("transaction", key, cipher, new Date(Date.now() + 1000));
    await new Promise((r) => setTimeout(r, 1100));
    assert.equal(await store.take("transaction", key), null);
    await assert.rejects(
      store.put(
        "transaction",
        randomKey(),
        cipher,
        new Date(Date.now() + 3600000),
      ),
    );
    await store.put(
      "session",
      randomKey(),
      cipher,
      new Date(Date.now() + 60000),
    );
    await assert.rejects(
      db.query(
        readFileSync(
          "../../migrations/000076_acceptance_browser_session.down.sql",
          "utf8",
        ),
      ),
    );
    await db.query("DELETE FROM platform_browser_session");
    await db.query(
      readFileSync(
        "../../migrations/000076_acceptance_browser_session.down.sql",
        "utf8",
      ),
    );
  },
);

test(
  "real PG down locks serialize active-state writer before drain check",
  { skip: !process.env.IAM_TEST_POSTGRES_DSN },
  async (t) => {
    const raw = process.env.IAM_TEST_POSTGRES_DSN,
      admin = new pg.Pool({ connectionString: raw }),
      schema = `browser_iam_${randomUUID().replaceAll("-", "")}`;
    await admin.query(`CREATE SCHEMA ${schema}`);
    const pool = new pg.Pool({
      connectionString: raw,
      options: `-csearch_path=${schema}`,
    });
    await pool.query(
      readFileSync(
        "../../migrations/000076_acceptance_browser_session.up.sql",
        "utf8",
      ),
    );
    const writer = await pool.connect(),
      down = await pool.connect();
    t.after(async () => {
      await writer.query("ROLLBACK");
      await down.query("ROLLBACK");
      writer.release();
      down.release();
      await pool.end();
      await admin.query(`DROP SCHEMA ${schema} CASCADE`);
      await admin.end();
    });
    const writerPID = (await writer.query("SELECT pg_backend_pid() AS pid"))
        .rows[0].pid,
      downPID = (await down.query("SELECT pg_backend_pid() AS pid")).rows[0]
        .pid;
    const sql = readFileSync(
      "../../migrations/000076_acceptance_browser_session.down.sql",
      "utf8",
    );
    const insert =
      "INSERT INTO platform_browser_session(key_hash,encrypted_state,expires_at) VALUES($1,$2,statement_timestamp()+interval '1 minute')";
    const waitBlocked = async (pid, blocker) => {
      for (let n = 0; n < 100; n++) {
        const result = await pool.query(
          "SELECT pg_blocking_pids($1) AS blockers",
          [pid],
        );
        if (result.rows[0].blockers.includes(blocker)) return;
        await new Promise((r) => setTimeout(r, 10));
      }
      throw new Error("expected actual PostgreSQL lock wait");
    };
    await writer.query("BEGIN");
    await writer.query(insert, [Buffer.alloc(32, 1), Buffer.alloc(33, 2)]);
    await down.query("BEGIN");
    const attempt = down.query(sql).then(
      () => "allowed",
      () => "rejected",
    );
    await waitBlocked(downPID, writerPID);
    await writer.query("COMMIT");
    assert.equal(await attempt, "rejected");
    await down.query("ROLLBACK");
    assert.equal(
      (
        await pool.query(
          "SELECT count(*)::int AS count FROM platform_browser_session",
        )
      ).rows[0].count,
      1,
    );
    await pool.query("DELETE FROM platform_browser_session");
    await down.query("BEGIN");
    await down.query(sql.slice(0, sql.indexOf("DO $$")));
    const late = writer
      .query(insert, [Buffer.alloc(32, 3), Buffer.alloc(33, 4)])
      .then(
        () => "accepted",
        () => "rejected",
      );
    await waitBlocked(writerPID, downPID);
    await down.query(sql);
    await down.query("COMMIT");
    assert.equal(await late, "rejected");
  },
);

test("acceptance transport admits only explicit user workspace GET; no writes, credentials, foreign paths or fallback",async(t)=>{
 const f=await fixture(t);let calls=0;
 f.state.businessHandler=(req,res)=>{calls++;assert.equal(req.headers.authorization,"Bearer access-one");res.end('{}');};
 const start=await f.iam.start(),session=await f.iam.callback(f.callback(start),start.key),client=await f.iam.acceptanceClient(session.key);
 const root=client.apiBaseUrl+`/api/v1/workspaces/${f.cfg.workspaceId}`;
 await client.request(root+'/acceptance-context');assert.equal(calls,1);
 for(const [url,init] of [[root+'/business-acceptances',{method:'POST',body:'{}'}],[root+'/acceptance-context',{method:'POST'}],[root+'/acceptance-context',{headers:{Authorization:'Bearer other'}}],[client.apiBaseUrl+'/api/v1/workspaces/44444444-4444-4444-8444-444444444444/acceptance-context',{}],['https://foreign.example.test/api/v1/workspaces/'+f.cfg.workspaceId+'/acceptance-context',{}]])await assert.rejects(client.request(url,init));
 assert.equal(calls,1);f.state.deny=true;await assert.rejects(client.request(root+'/acceptance-context'));assert.equal(calls,1);
});

test("issuer, client, origin or workspace profile changes invalidate an existing session",async(t)=>{
 const f=await fixture(t);const start=await f.iam.start(),s=await f.iam.callback(f.callback(start),start.key);
 for(const changed of [{origin:'https://other.example.test'},{clientId:'another-rp'},{issuer:'https://another.example.test'},{workspaceId:'44444444-4444-4444-8444-444444444444'}]){
  const other=new BrowserIAM({...f.cfg,...changed},f.store);await assert.rejects(other.current(s.key));
 }
});
