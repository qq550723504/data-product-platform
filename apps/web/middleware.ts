import {NextRequest,NextResponse} from 'next/server';
export function middleware(r:NextRequest){
 const mode=process.env.BROWSER_IAM_MODE;
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
