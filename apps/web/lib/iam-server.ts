import "server-only";
import { Pool } from "pg";
import { cookies, headers } from "next/headers";
import {
  BrowserIAM,
  cookieNames,
  IAMError,
} from "./iam-core";
import { PostgresSessionStore } from "./iam-store";
import { iamEnabled } from "./iam-mode";
import { workspaceSelection } from "./iam-workspace";
import { browserConfiguration } from "./iam-configuration";

const instances=new Map<string,BrowserIAM>();
export async function browserIAM(workspaceOverride?:string): Promise<BrowserIAM> {
  if (!iamEnabled() || process.env.BROWSER_IAM_MODE !== "oidc")
    throw new IAMError();
  const selected=(await cookies()).get("__Host-dpp-workspace")?.value;
  const workspaceId=workspaceOverride??workspaceSelection(selected);
  const existing=instances.get(workspaceId);if(existing)return existing;
  const {config,databaseOptions} = browserConfiguration(workspaceId);
  const pool = new Pool(databaseOptions);
  const instance = new BrowserIAM(config, new PostgresSessionStore(pool));
  instances.set(workspaceId,instance);
  return instance;
}
export async function currentBrowserPrincipal() {
  if ((await headers()).has("authorization")) throw new IAMError();
  const key = (await cookies()).get(cookieNames.session)?.value;
  if (!key) throw new IAMError();
  return (await browserIAM()).current(key);
}

export async function currentAcceptanceClient(){
 if((await headers()).has("authorization"))throw new IAMError();
 const key=(await cookies()).get(cookieNames.session)?.value;if(!key)throw new IAMError();
 return (await browserIAM()).acceptanceClient(key);
}
