// Default off leaves existing Agent and preview transports unchanged.
export function iamEnabled(){return !!process.env.BROWSER_IAM_MODE && process.env.BROWSER_IAM_MODE!=="off";}
export type IAMProfile="business-acceptance"|"platform-management";
export function configuredIAMProfile():IAMProfile{const p=process.env.BROWSER_IAM_PROFILE;if(p!=="business-acceptance"&&p!=="platform-management")throw new Error("Browser identity profile unavailable");return p;}
