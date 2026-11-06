import { NextResponse } from 'next/server';
import { AUTH_COOKIE, cookieOptions, isSecureRequest } from '@/lib/auth/session';

export async function POST(req: Request) {
  const res = NextResponse.json({ ok: true });
  res.cookies.set(AUTH_COOKIE, '', { ...cookieOptions(isSecureRequest(req)), maxAge: 0 });
  return res;
}
