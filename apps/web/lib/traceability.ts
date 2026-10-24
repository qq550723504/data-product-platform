import {qualityRead} from "./quality-server";
import {configuredWorkspaceId} from "./platform";
import type {ReleaseTrace} from './release-trace';
export type {ReleaseTrace} from './release-trace';
export class TraceabilityError extends Error {
 constructor(message:string,readonly status:number,readonly code?:string){super(message);this.name='TraceabilityError';}
}
export async function productReleaseTrace(releaseId:string):Promise<ReleaseTrace>{
 const ws=configuredWorkspaceId();if(!ws)throw new TraceabilityError('Workspace is not configured',503);
 return qualityRead<ReleaseTrace>(`/api/v1/traceability/product-releases/${encodeURIComponent(releaseId)}?workspaceId=${encodeURIComponent(ws)}`);
}
