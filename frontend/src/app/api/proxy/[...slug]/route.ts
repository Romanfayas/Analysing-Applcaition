import { NextRequest, NextResponse } from 'next/server';
import { AUTH_COOKIE, BACKEND_API_URL, cookieOptions, isLive, isSecureRequest } from '@/lib/auth/session';

// Backend-for-frontend: the browser talks only to this route; the JWT is attached server-side
// from the httpOnly session cookie. Anything the client sends in Authorization/Cookie is discarded.

const FORWARDED_REQUEST_HEADERS = ['content-type', 'accept', 'accept-language'];

async function handleRequest(request: NextRequest, { params }: { params: Promise<{ slug: string[] }> }) {
  const token = request.cookies.get(AUTH_COOKIE)?.value;
  if (!token || !isLive(token)) {
    return NextResponse.json({ error: 'not authenticated' }, { status: 401 });
  }

  const { slug } = await params;
  // Normalised segments can't climb out of the API prefix.
  if (slug.some((s) => s === '..' || s === '.' || s.includes('/') || s.includes('\\'))) {
    return NextResponse.json({ error: 'invalid path' }, { status: 400 });
  }
  const targetUrl = `${BACKEND_API_URL}/${slug.map(encodeURIComponent).join('/')}${request.nextUrl.search}`;

  const headers = new Headers();
  for (const name of FORWARDED_REQUEST_HEADERS) {
    const v = request.headers.get(name);
    if (v) headers.set(name, v);
  }
  headers.set('Authorization', `Bearer ${token}`);

  const init: RequestInit = { method: request.method, headers, redirect: 'manual', cache: 'no-store' };
  if (!['GET', 'HEAD'].includes(request.method)) {
    init.body = await request.blob();
  }

  try {
    const upstream = await fetch(targetUrl, init);
    const body = await upstream.blob();
    const responseHeaders = new Headers();
    const contentType = upstream.headers.get('content-type');
    if (contentType) responseHeaders.set('content-type', contentType);
    responseHeaders.set('Cache-Control', 'no-store');
    const response = new NextResponse(body, { status: upstream.status, headers: responseHeaders });
    if (upstream.status === 401) {
      // The backend rejected the session (expired, or the account was deleted/disabled). The edge gate only reads the cookie's
      // expiry date, so without this the browser would bounce /login -> / -> 401 forever. Drop the cookie so /login is reachable.
      response.cookies.set(AUTH_COOKIE, '', { ...cookieOptions(isSecureRequest(request)), maxAge: 0, expires: new Date(0) });
    }
    return response;
  } catch (error) {
    console.error('Proxy error:', error);
    return NextResponse.json({ error: 'Proxy request to backend failed' }, { status: 502 });
  }
}

export const GET = handleRequest;
export const POST = handleRequest;
export const PUT = handleRequest;
export const DELETE = handleRequest;
export const PATCH = handleRequest;
