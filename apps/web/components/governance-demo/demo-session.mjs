// Session-scoped synthetic UI state only. No Core identity, approval or intent is stored here.
export const demoStorageKeys={registry:'dpp:governance-demo:registry:v1',data:'dpp:governance-demo:data:v1'};
export function readDemoSession(key,fallback){try{const raw=sessionStorage.getItem(key);if(!raw)return fallback;const value=JSON.parse(raw);if(!value||typeof value!=='object'||Array.isArray(value))return fallback;if(key===demoStorageKeys.registry&&(!Array.isArray(value.items)||!value.items.some(w=>w.id===value.currentId)))return fallback;return value}catch{return fallback}}
export function saveDemoSession(key,value){try{sessionStorage.setItem(key,JSON.stringify(value));return true}catch{return false}}
export function resetDemoSession(){for(const key of Object.values(demoStorageKeys))sessionStorage.removeItem(key);location.assign('/#home')}
