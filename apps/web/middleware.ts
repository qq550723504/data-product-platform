import {NextRequest,NextResponse} from 'next/server';
export function middleware(request:NextRequest){
 // Only the public prototype homepage is rewritten. Existing server-backed
 // resource/version/product URLs retain their exact query and object context.
 if(request.nextUrl.pathname!=="/")return NextResponse.next();
 const url=request.nextUrl.clone();url.pathname='/demo';return NextResponse.rewrite(url);
}
export const config={matcher:['/((?!api(?:/|$)|_next(?:/|$)|design(?:/|$)|favicon.ico|.*\\.[a-zA-Z0-9]+$).*)']};
