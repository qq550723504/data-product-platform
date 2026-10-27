import { readFileSync } from "node:fs";
import { IAMError, validateConfig, type IAMConfig } from "./iam-core";
import { configuredIAMProfile } from "./iam-mode";
import { verifiedDatabaseOptions } from "./iam-store";

// Runtime and offline preflight share this loader. It constructs no pool,
// performs no discovery/network request, and never exposes underlying errors.
export function browserConfiguration(workspaceId?:string) {
  try {
    if (process.env.BROWSER_IAM_MODE !== "oidc") throw new IAMError();
    const env = (name: string) => process.env[`BROWSER_IAM_${name}`]?.trim() ?? "";
    const encoded = env("ENCRYPTION_KEY");
    const backchannel = env('BACKCHANNEL_LOGOUT_ENABLED');
    if(backchannel && !['true','false'].includes(backchannel)) throw new IAMError();
    if (!/^[A-Za-z0-9_-]{43}$/.test(encoded)) throw new IAMError();
    const config: IAMConfig = {
      profile: configuredIAMProfile(), annotationWorkspaceId: env("ANNOTATION_WORKSPACE_ID") || undefined,
      backchannelLogoutEnabled: backchannel === 'true',
      origin: env("ORIGIN"), issuer: env("ISSUER"), clientId: env("CLIENT_ID"), clientSecret: env("CLIENT_SECRET"),
      authorizationEndpoint: env("AUTHORIZATION_ENDPOINT"), tokenEndpoint: env("TOKEN_ENDPOINT"), jwksURI: env("JWKS_URI"),
      logoutEndpoint: env("LOGOUT_ENDPOINT") || undefined, principalEndpoint: env("PRINCIPAL_ENDPOINT"),
      workspaceId: workspaceId ?? env("WORKSPACE_ID"), scope: env("SCOPE"), encryptionKey: Buffer.from(encoded,"base64url"),
    };
    validateConfig(config);
    const databaseOptions = verifiedDatabaseOptions(env("DATABASE_URL"),env("DATABASE_CA_FILE") ? readFileSync(env("DATABASE_CA_FILE"),"utf8") : undefined);
    return {config,databaseOptions};
  } catch { throw new IAMError(); }
}
