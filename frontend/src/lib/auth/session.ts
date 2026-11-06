// Server-side session helpers. The backend JWT lives only in an httpOnly cookie, so page scripts
// (and therefore any XSS) can never read it; the proxy route attaches it to backend calls.

export const AUTH_COOKIE = 'he_session';

export const BACKEND_API_URL = process.env.BACKEND_API_URL || 'http://backend:8080/api/v1';

export interface SessionUser {
  id: string;
  email: string;
  name: string;
}

interface JwtPayload {
  user_id?: string;
  email?: string;
  name?: string;
  exp?: number;
}

/** Decodes (does NOT verify) a JWT payload. Verification is the backend's job on every request;
 *  this is only used to read display fields and to skip obviously expired sessions early. */
export function decodeJwt(token: string | undefined): JwtPayload | null {
  if (!token) return null;
  const parts = token.split('.');
  if (parts.length !== 3) return null;
  try {
    const b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    const json = atob(b64.padEnd(b64.length + ((4 - (b64.length % 4)) % 4), '='));
    return JSON.parse(json) as JwtPayload;
  } catch {
    return null;
  }
}

export function isLive(token: string | undefined): boolean {
  const payload = decodeJwt(token);
  return !!payload?.exp && payload.exp * 1000 > Date.now();
}

export function userFromToken(token: string | undefined): SessionUser | null {
  const p = decodeJwt(token);
  if (!p?.user_id || !p.email || !isLive(token)) return null;
  return { id: p.user_id, email: p.email, name: p.name ?? p.email };
}

export function cookieOptions(secure: boolean, expiresAt?: string) {
  const expires = expiresAt ? new Date(expiresAt) : undefined;
  return {
    httpOnly: true,
    sameSite: 'lax' as const,
    secure,
    path: '/',
    ...(expires && !Number.isNaN(expires.getTime()) ? { expires } : {}),
  };
}

export function isSecureRequest(req: Request): boolean {
  const proto = req.headers.get('x-forwarded-proto') ?? new URL(req.url).protocol.replace(':', '');
  return proto === 'https';
}
