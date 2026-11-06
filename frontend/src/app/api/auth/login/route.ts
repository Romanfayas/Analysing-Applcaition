import { forwardAuth } from '@/lib/auth/forward';

export async function POST(req: Request) {
  return forwardAuth(req, 'login');
}
