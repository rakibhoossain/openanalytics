import { createHash } from 'node:crypto';
import { NextResponse } from 'next/server.js';

type RouteHandlerOptions = {
  apiUrl?: string;
  tenantId?: string;
  shopId?: string;
};

const DEFAULT_API_URL = 'http://127.0.0.1:8080';
const SCRIPT_URL = 'http://127.0.0.1:8080';
const SCRIPT_PATH = '/oa.js';

function getClientHeaders(req: Request, options?: RouteHandlerOptions): Headers {
  const headers = new Headers();
  const ip =
    req.headers.get('cf-connecting-ip') ??
    req.headers.get('x-forwarded-for')?.split(',')[0] ??
    req.headers.get('x-vercel-forwarded-for') ??
    req.headers.get('x-real-ip');

  headers.set('Content-Type', 'application/json');

  const clientId =
    req.headers.get('openpanel-client-id') ??
    req.headers.get('openanalytics-client-id') ??
    req.headers.get('x-shop-id') ??
    options?.shopId ??
    '';
  if (clientId) {
    headers.set('openpanel-client-id', clientId);
    headers.set('X-Shop-Id', clientId);
    headers.set('openanalytics-client-id', clientId);
  }

  const tenantId = req.headers.get('x-tenant-id') ?? options?.tenantId;
  if (tenantId) {
    headers.set('X-Tenant-Id', tenantId);
  }

  const origin =
    req.headers.get('origin') ??
    (() => {
      try {
        const url = new URL(req.url);
        return `${url.protocol}//${url.host}`;
      } catch {
        return '';
      }
    })();
  headers.set('origin', origin);
  headers.set('User-Agent', req.headers.get('user-agent') ?? '');
  if (ip) {
    headers.set('openpanel-client-ip', ip);
    headers.set('X-Forwarded-For', ip);
  }

  return headers;
}

async function handleApiRoute(
  req: Request,
  apiUrl: string,
  apiPath: string,
  options?: RouteHandlerOptions,
): Promise<NextResponse> {
  const headers = getClientHeaders(req, options);

  try {
    const res = await fetch(`${apiUrl}${apiPath}`, {
      method: req.method,
      headers,
      body:
        req.method === 'POST' ? JSON.stringify(await req.json()) : undefined,
    });

    if (res.headers.get('content-type')?.includes('application/json')) {
      return NextResponse.json(await res.json(), { status: res.status });
    }
    return NextResponse.json(await res.text(), { status: res.status });
  } catch (e) {
    return NextResponse.json(
      {
        error: 'Failed to proxy request',
        message: e instanceof Error ? e.message : String(e),
      },
      { status: 500 },
    );
  }
}

async function handleScriptProxyRoute(req: Request): Promise<NextResponse> {
  const url = new URL(req.url);
  const pathname = url.pathname;

  if (!pathname.endsWith(SCRIPT_PATH) && !pathname.endsWith('/op1.js')) {
    return NextResponse.json({ error: 'Not found' }, { status: 404 });
  }

  let scriptUrl = `${SCRIPT_URL}${pathname.endsWith('/op1.js') ? '/op1.js' : SCRIPT_PATH}`;
  if (url.searchParams.size > 0) {
    scriptUrl += `?${url.searchParams.toString()}`;
  }

  try {
    const res = await fetch(scriptUrl, {
      // @ts-ignore
      next: { revalidate: 86400 },
    });
    const text = await res.text();
    const etag = `"${createHash('md5')
      .update(scriptUrl + text)
      .digest('hex')}"`;

    return new NextResponse(text, {
      headers: {
        'Content-Type': 'text/javascript',
        'Cache-Control': 'public, max-age=86400, stale-while-revalidate=86400',
        ETag: etag,
      },
    });
  } catch (e) {
    return NextResponse.json(
      {
        error: 'Failed to fetch script',
        message: e instanceof Error ? e.message : String(e),
      },
      { status: 500 },
    );
  }
}

function createRouteHandler(options?: RouteHandlerOptions) {
  const apiUrl = options?.apiUrl ?? DEFAULT_API_URL;

  const handler = async function handler(req: Request): Promise<NextResponse> {
    const url = new URL(req.url);
    const pathname = url.pathname;
    const method = req.method;

    if (method === 'GET' && (pathname.endsWith(SCRIPT_PATH) || pathname.endsWith('/op1.js'))) {
      return handleScriptProxyRoute(req);
    }

    if (pathname.includes('/track')) {
      const apiPathMatch = pathname.indexOf('/track');
      const apiPath = '/api/v1' + pathname.substring(apiPathMatch);
      return handleApiRoute(req, apiUrl, apiPath, options);
    }

    if (pathname.includes('/replay')) {
      return handleApiRoute(req, apiUrl, '/api/v1/replay', options);
    }

    return NextResponse.json({ error: 'Not found' }, { status: 404 });
  };

  handler.GET = handler;
  handler.POST = handler;

  return handler;
}

export { createRouteHandler };
