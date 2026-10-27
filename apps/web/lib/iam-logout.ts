import { createHash } from 'node:crypto';
import { createRemoteJWKSet, customFetch, jwtVerify } from 'jose';
import type { IAMConfig } from './iam-core';

export type ProviderSessionBinding = {scope: Buffer; subject?: Buffer};
const event = 'http://schemas.openid.net/event/backchannel-logout';
const identifier = (v: unknown): v is string => typeof v === 'string' && !!v.trim() && v.length <= 256;
const digest = (v: string[]) => createHash('sha256').update(JSON.stringify(v)).digest();
export function providerSession(c: IAMConfig, sid: unknown, subject?: unknown): ProviderSessionBinding {
  if (!identifier(sid) || subject !== undefined && !identifier(subject)) throw new Error('Invalid provider session');
  return {scope: digest(['oidc-session', c.issuer, c.clientId, sid]),
    ...(subject !== undefined ? {subject: digest(['oidc-subject', c.issuer, subject as string])} : {})};
}
export function logoutValidator(c: IAMConfig, request: typeof fetch) {
  const keys = createRemoteJWKSet(new URL(c.jwksURI), {[customFetch]:request, timeoutDuration:10000});
  return async (value: string) => {
    if (!value || value.length > 8192) throw new Error('Invalid logout notification');
    const {payload:p, protectedHeader:h} = await jwtVerify(value, keys, {
      algorithms:['RS256'], issuer:c.issuer, audience:c.clientId,
      maxTokenAge:300, clockTolerance:0,
      requiredClaims:['iss','aud','iat','exp','jti','sid','events'],
    });
    const events = p.events;
    if (h.typ !== undefined && !['JWT','logout+jwt'].includes(h.typ) || h.jku || h.x5u ||
        !identifier(p.jti) || 'nonce' in p || !events || typeof events !== 'object' || Array.isArray(events) ||
        !(event in events) || !(events as Record<string, unknown>)[event] ||
        typeof (events as Record<string, unknown>)[event] !== 'object' ||
        Array.isArray((events as Record<string, unknown>)[event])) throw new Error('Invalid logout notification');
    return providerSession(c, p.sid, p.sub);
  };
}

export async function readLogoutForm(request: Request): Promise<string> {
  if (request.headers.get('content-type')?.split(';')[0].trim().toLowerCase() !== 'application/x-www-form-urlencoded') throw new Error('Invalid logout notification');
  const length = request.headers.get('content-length');
  if (length !== null && (!/^[0-9]+$/.test(length) || Number(length)>16384)) throw new Error('Invalid logout notification');
  const reader = request.body?.getReader();
  if (!reader) throw new Error('Invalid logout notification');
  const chunks: Uint8Array[] = []; let size=0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const deadline = new Promise<never>((_,reject) => {timer=setTimeout(()=>reject(new Error('Invalid logout notification')),10000)});
  try {
    for (;;) {
      const {done,value} = await Promise.race([reader.read(),deadline]); if(done) break;
      size+=value.length; if(size>16384) throw new Error('Invalid logout notification'); chunks.push(value);
    }
    const form = new URLSearchParams(new TextDecoder('utf-8',{fatal:true}).decode(Buffer.concat(chunks)));
    if(form.getAll('logout_token').length!==1) throw new Error('Invalid logout notification');
    return form.get('logout_token')!;
  } finally { clearTimeout(timer); await reader.cancel(); }
}
