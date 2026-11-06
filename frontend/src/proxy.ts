import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';
import { AUTH_COOKIE, isLive } from '@/lib/auth/session';

// Optimistic gate only: it redirects visitors without a live-looking session to /login.
// Real authorization happens in the backend, which validates the JWT on every API call.
export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const authed = isLive(request.cookies.get(AUTH_COOKIE)?.value);

  if (pathname === '/login') {
    return authed ? NextResponse.redirect(new URL('/', request.url)) : NextResponse.next();
  }
  if (!authed) {
    const url = new URL('/login', request.url);
    if (pathname !== '/') url.searchParams.set('next', pathname);
    return NextResponse.redirect(url);
  }
  return NextResponse.next();
}

export const config = {
  // Everything except API routes (which authorise themselves), Next internals and static files.
  matcher: ['/((?!api/|_next/|favicon.ico|.*\\.(?:png|jpg|jpeg|svg|ico|webp|woff2?)$).*)'],
};
