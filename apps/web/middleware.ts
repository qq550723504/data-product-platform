import {NextRequest,NextResponse} from 'next/server';
import settingsInteractionRoutes from './lib/settings-interaction-routes.json';
export function middleware(r:NextRequest){
 const mode=process.env.BROWSER_IAM_MODE;
 if(process.env.GOVERNANCE_SETTINGS_INTERACTION_DEMO==='true'){
  if(mode&&mode!=='off')return new NextResponse('Interaction preview cannot use connected identity',{status:503,headers:{'Cache-Control':'no-store'}});
  const path=r.nextUrl.pathname;
  if(!['GET','HEAD'].includes(r.method)||['/api','/auth','/core','/agent'].some(p=>path===p||path.startsWith(p+'/')))return new NextResponse('Local interaction preview only',{status:403,headers:{'Cache-Control':'no-store'}});
  const settings:Record<string,string>=settingsInteractionRoutes;
  if(settings[path]){
   const host=r.headers.get('host')??'';
   if(!/^127\.0\.0\.1:\d+$/.test(host))return new NextResponse('Loopback interaction preview required',{status:403});
   return NextResponse.redirect(new URL('/#'+settings[path],'http://'+host));
  }
  if(path==='/'){const u=r.nextUrl.clone();u.pathname='/demo';return NextResponse.rewrite(u)}
  if(path==='/demo'||path.startsWith('/_next/')||path.startsWith('/design/')||path==='/favicon.ico')return NextResponse.next();
  return new NextResponse('Local interaction preview route unavailable',{status:404,headers:{'Cache-Control':'no-store'}});
 }
 if(!mode||mode==='off'){
  // Preserve the existing public prototype rewrite and all exact Core URLs.
  if(r.nextUrl.pathname!=="/")return NextResponse.next();
  const url=r.nextUrl.clone();url.pathname='/demo';return NextResponse.rewrite(url);
 }
 const profile=process.env.BROWSER_IAM_PROFILE;
 if(mode!=='oidc'||!['business-acceptance','platform-management'].includes(profile??''))return new NextResponse('Identity profile unavailable',{status:503,headers:{'Cache-Control':'no-store'}});
 const path=r.nextUrl.pathname;
 if(path==='/'&&r.method==='GET')return NextResponse.redirect(new URL(profile==='platform-management'?'/settings':'/identity',r.url));
 if((r.method==='GET'||r.method==='HEAD')&&(path.startsWith('/_next/static/')||path==='/favicon.ico'||profile==='platform-management'&&path==='/design/brand.png'))return NextResponse.next();
 const auth=['/auth/login','/auth/callback','/auth/logout','/auth/logged-out'];
 const pages=profile==='platform-management'?['/identity','/settings','/settings/members','/settings/roles','/settings/audit','/settings/workspaces','/settings/users','/settings/organizations','/settings/logs','/settings/configuration']:['/identity','/core/acceptance-context','/core/acceptance-basis'];
 if(auth.includes(path)||r.method==='GET'&&pages.includes(path))return NextResponse.next();
 return new NextResponse('This identity profile provides acceptance reads only',{status:403,headers:{'Cache-Control':'no-store'}});
}
