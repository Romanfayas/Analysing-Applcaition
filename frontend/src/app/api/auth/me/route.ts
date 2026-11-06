import { NextResponse } from 'next/server';
import { cookies } from 'next/headers';
import { AUTH_COOKIE, userFromToken } from '@/lib/auth/session';

export async function GET() {
  const token = (await cookies()).get(AUTH_COOKIE)?.value;
  const user = userFromToken(token);
  if (!user) return NextResponse.json({ error: 'not authenticated' }, { status: 401 });
  return NextResponse.json({ user }, { headers: { 'Cache-Control': 'no-store' } });
}
