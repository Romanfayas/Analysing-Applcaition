import { NextRequest, NextResponse } from 'next/server';

const API_BASE_URL = process.env.BACKEND_API_URL || 'http://backend:8080/api/v1';

async function handleRequest(
  request: NextRequest,
  { params }: { params: Promise<{ slug: string[] }> }
) {
  if (process.env.APP_ENV === 'production') {
    return NextResponse.json(
      { error: 'Development proxy is strictly disabled in production environments.' },
      { status: 403 }
    );
  }

  const { slug } = await params;
  const path = slug.join('/');
  const targetUrl = `${API_BASE_URL}/${path}${request.nextUrl.search}`;

  const headers = new Headers(request.headers);
  headers.delete('host');
  headers.delete('connection');
  headers.delete('accept-encoding');

  const devToken = process.env.DEV_JWT || 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiMDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAxIiwiZW1haWwiOiJkZXZAaGFsYWxlcXVpdHkubG9jYWwiLCJuYW1lIjoiRGV2ZWxvcGVyIiwiaXNzIjoiaGFsYWwtZXF1aXR5LXBsYXRmb3JtIiwiZXhwIjoxODIzMzMzMjcxLCJpYXQiOjE3OTE3OTcyNzF9.Z4UjUvH3BdXm-xM0kpj1JuJm8-vpU2yZOOxt2tAtkeo';
  if (!headers.get('Authorization') && devToken) {
    headers.set('Authorization', `Bearer ${devToken}`);
  }

  const init: RequestInit = {
    method: request.method,
    headers,
    redirect: 'manual',
  };

  if (['POST', 'PUT', 'PATCH'].includes(request.method)) {
    try {
      const clonedReq = request.clone();
      init.body = await clonedReq.blob();
    } catch (e) {
      // Body missing or unparseable, ignore
    }
  }

  try {
    const response = await fetch(targetUrl, init);
    const data = await response.blob();

    const responseHeaders = new Headers();
    response.headers.forEach((value, key) => {
      // Don't forward content-encoding to avoid double-compression issues
      if (key.toLowerCase() !== 'content-encoding') {
        responseHeaders.set(key, value);
      }
    });

    return new NextResponse(data, {
      status: response.status,
      statusText: response.statusText,
      headers: responseHeaders,
    });
  } catch (error) {
    console.error('Proxy Error:', error);
    return NextResponse.json({ error: 'Proxy request to backend failed' }, { status: 502 });
  }
}

export const GET = handleRequest;
export const POST = handleRequest;
export const PUT = handleRequest;
export const DELETE = handleRequest;
export const PATCH = handleRequest;
