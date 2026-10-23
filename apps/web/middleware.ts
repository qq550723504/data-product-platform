import {NextRequest,NextResponse} from 'next/server';
export function middleware(request:NextRequest){
 const path=request.nextUrl.pathname;
 if(["/core/assets","/core/certifications","/core/releases","/core/release-trace"].includes(path)||path==="/demo"||path==="/agent"||path.startsWith('/agent/'))return NextResponse.next();
 const url=request.nextUrl.clone();url.pathname='/demo';
 return NextResponse.rewrite(url);
}
export const config={matcher:['/((?!api(?:/|$)|_next(?:/|$)|design(?:/|$)|favicon.ico|.*\\.[a-zA-Z0-9]+$).*)']};
