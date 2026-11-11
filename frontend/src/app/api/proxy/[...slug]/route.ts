import { gzipSync } from 'node:zlib';
import { NextRequest, NextResponse } from 'next/server';
import { AUTH_COOKIE, BACKEND_API_URL, cookieOptions, isLive, isSecureRequest } from '@/lib/auth/session';

// Backend-for-frontend: the browser talks only to this route; the JWT is attached server-side
// from the httpOnly session cookie. Anything the client sends in Authorization/Cookie is discarded.

const FORWARDED_REQUEST_HEADERS = ['content-type', 'accept', 'accept-language'];

// Read-only endpoints that each cost 0.7-2.5 s on the server (many queries across the database tunnel). Their data changes at most every
// minute, so a few seconds of per-session caching, with concurrent requests sharing one call, makes every repeat visit instant.
const CACHEABLE = new Set(['stocks', 'signals', 'data-quality/intraday-feed', 'breakouts/overview', 'breakouts/upcoming', 'signals/breakouts', 'signals/breakouts/monitor', 'learning/summary']);
const CACHE_TTL_MS = 20_000;
const cache = new Map<string, { at: number; pending?: Promise<{ status: number; type: string | null; body: Buffer }>; value?: { status: number; type: string | null; body: Buffer } }>();

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

  const call = async () => {
    const r = await fetch(targetUrl, init);
    return { status: r.status, type: r.headers.get('content-type'), body: Buffer.from(await r.arrayBuffer()) };
  };
  try {
    let result: { status: number; type: string | null; body: Buffer };
    const cacheKey = `${token}|${slug.join('/')}${request.nextUrl.search}`;
    if (request.method === 'GET' && CACHEABLE.has(slug.join('/'))) {
      const hit = cache.get(cacheKey);
      if (hit && Date.now() - hit.at < CACHE_TTL_MS && (hit.value || hit.pending)) {
        result = hit.value ?? (await hit.pending!);
      } else {
        const entry: NonNullable<ReturnType<typeof cache.get>> = { at: Date.now() };
        entry.pending = call();
        cache.set(cacheKey, entry);
        try {
          result = await entry.pending;
          if (result.status === 200) entry.value = result; else cache.delete(cacheKey);
        } catch (e) {
          cache.delete(cacheKey);
          throw e;
        }
        if (cache.size > 200) for (const [k, v] of cache) if (Date.now() - v.at > CACHE_TTL_MS) cache.delete(k);
      }
    } else {
      result = await call();
    }
    const upstream = { status: result.status };
    const raw = result.body;
    const responseHeaders = new Headers();
    const contentType = result.type;
    if (contentType) responseHeaders.set('content-type', contentType);
    responseHeaders.set('Cache-Control', 'no-store');
    // Next does not compress route-handler responses. Behind the public proxy a 94 KB JSON took ~4 s to arrive; gzipped it is ~10 KB.
    let body: Uint8Array = raw;
    if (raw.length > 1024 && /\bgzip\b/.test(request.headers.get('accept-encoding') ?? '') && /json|text/.test(contentType ?? '')) {
      body = gzipSync(raw, { level: 6 });
      responseHeaders.set('Content-Encoding', 'gzip');
      responseHeaders.set('Vary', 'Accept-Encoding');
    }
    const response = new NextResponse(body as BodyInit, { status: upstream.status, headers: responseHeaders });
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
