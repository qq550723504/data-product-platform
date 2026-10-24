// Default off leaves existing Agent and preview transports unchanged.
export function iamEnabled(){return !!process.env.BROWSER_IAM_MODE && process.env.BROWSER_IAM_MODE!=="off";}
export type IAMProfile="business-acceptance";
export function configuredIAMProfile():IAMProfile{if(process.env.BROWSER_IAM_PROFILE!=="business-acceptance")throw new Error("Browser identity profile unavailable");return "business-acceptance";}
