import { NextResponse, type NextRequest } from 'next/server';
import { sessionCookie, signedIn } from '@/lib/auth';

// Everything but the sign-in page needs the passcode's cookie: pages go to
// the sign-in page, and API calls get a 401.
export async function proxy(req: NextRequest) {
  if (await signedIn(req.cookies.get(sessionCookie)?.value)) {
    return NextResponse.next();
  }
  if (req.nextUrl.pathname.startsWith('/api/') || req.nextUrl.pathname.startsWith('/img/')) {
    return new NextResponse('Sign in first.', { status: 401 });
  }
  const login = new URL('/login', req.url);
  return NextResponse.redirect(login);
}

export const config = {
  matcher: ['/((?!login|api/login|_next/static|_next/image|favicon.ico).*)'],
};
