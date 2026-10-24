import {NextRequest,NextResponse} from 'next/server';
export function middleware(r:NextRequest){
 const mode=process.env.BROWSER_IAM_MODE;
 if(!mode||mode==='off'){
  // Preserve the existing public prototype rewrite and all exact Core URLs.
  if(r.nextUrl.pathname!=="/")return NextResponse.next();
  const url=r.nextUrl.clone();url.pathname='/demo';return NextResponse.rewrite(url);
 }
 if(mode!=='oidc'||process.env.BROWSER_IAM_PROFILE!=='business-acceptance')return new NextResponse('Identity profile unavailable',{status:503,headers:{'Cache-Control':'no-store'}});
 const path=r.nextUrl.pathname;
 if(path==='/'&&r.method==='GET')return NextResponse.redirect(new URL('/identity',r.url));
 if((r.method==='GET'||r.method==='HEAD')&&(path.startsWith('/_next/static/')||path==='/favicon.ico'))return NextResponse.next();
 const auth=['/auth/login','/auth/callback','/auth/logout','/auth/logged-out'];
 if(auth.includes(path)||r.method==='GET'&&(path==='/identity'||path==='/core/acceptance-context'))return NextResponse.next();
 return new NextResponse('This identity profile provides acceptance reads only',{status:403,headers:{'Cache-Control':'no-store'}});
}
