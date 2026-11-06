import { NextResponse } from 'next/server';
import { AUTH_COOKIE, BACKEND_API_URL, cookieOptions, isSecureRequest, type SessionUser } from './session';

interface AuthPayload {
  data?: { token: string; expires_at: string; user: SessionUser };
  error?: string;
}

/** Forwards a credential request to the backend and, on success, stores the token in an httpOnly cookie.
 *  The token is never returned to the browser. */
export async function forwardAuth(req: Request, backendPath: 'login' | 'register'): Promise<NextResponse> {
  let body: unknown;
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: 'invalid request body' }, { status: 422 });
  }

  let res: Response;
  try {
    res = await fetch(`${BACKEND_API_URL}/auth/${backendPath}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      cache: 'no-store',
    });
  } catch {
    return NextResponse.json({ error: 'authentication service unavailable' }, { status: 502 });
  }

  const payload = (await res.json().catch(() => ({}))) as AuthPayload;
  if (!res.ok || !payload.data?.token) {
    return NextResponse.json({ error: payload.error ?? 'authentication failed' }, { status: res.status >= 400 ? res.status : 502 });
  }

  const out = NextResponse.json({ user: payload.data.user }, { status: res.status });
  out.cookies.set(AUTH_COOKIE, payload.data.token, cookieOptions(isSecureRequest(req), payload.data.expires_at));
  return out;
}
