// Optional front door: gives the app a short address (https://<name>.<account>.workers.dev, or your own domain) and caches the hashed
// /_next/static files for a year, which the Instabox proxy otherwise marks "no-store". Free Cloudflare plan is enough.
// Setup: Cloudflare dashboard > Workers & Pages > Create > Hello World > paste this file > Deploy; name the worker (that is the address).
// Then set ORIGIN below to the Instabox URL of server 1.
const ORIGIN = 'https://3000-agq4juf6ixalcsity3la2ldyam.instabox.tech';

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    const target = new URL(url.pathname + url.search, ORIGIN);
    const isStatic = request.method === 'GET' && url.pathname.startsWith('/_next/static/');
    const cache = caches.default;
    if (isStatic) {
      const hit = await cache.match(request);
      if (hit) return hit;
    }
    const headers = new Headers(request.headers);
    headers.set('x-forwarded-host', url.host);
    headers.set('x-forwarded-proto', 'https');
    const upstream = await fetch(target, { method: request.method, headers, body: ['GET', 'HEAD'].includes(request.method) ? undefined : request.body, redirect: 'manual' });
    const out = new Response(upstream.body, upstream);
    const loc = out.headers.get('location');
    if (loc && loc.startsWith(ORIGIN)) out.headers.set('location', loc.replace(ORIGIN, url.origin)); // keep redirects on the short address
    if (isStatic && upstream.status === 200) {
      out.headers.set('cache-control', 'public, max-age=31536000, immutable');
      ctx.waitUntil(cache.put(request, out.clone()));
    }
    return out;
  },
};
