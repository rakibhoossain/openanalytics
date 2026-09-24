import { describe, expect, it, vi, beforeEach } from 'vitest';
import { createRouteHandler } from './createNextRouteHandler';

describe('Next.js Route Handler Proxy', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('proxies POST requests to backend with client IP and shop headers', async () => {
    const handler = createRouteHandler({ apiUrl: 'http://backend.internal' });

    let proxiedUrl = '';
    let proxiedHeaders: any = null;
    let proxiedBody = '';

    globalThis.fetch = vi.fn().mockImplementation(async (url: string, init: any) => {
      proxiedUrl = url;
      proxiedHeaders = init.headers;
      proxiedBody = init.body;
      return {
        status: 202,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ status: 'accepted' }),
      } as any;
    });

    const req = new Request('http://localhost:3000/api/analytics/track', {
      method: 'POST',
      headers: {
        'cf-connecting-ip': '203.0.113.195',
        'openpanel-client-id': '01924b12-0000-7000-8000-000000000001',
        'user-agent': 'Mozilla/5.0 TestBrowser',
      },
      body: JSON.stringify({ name: 'click_btn' }),
    });

    const response = await handler(req);
    expect(response.status).toBe(202);
    expect(proxiedUrl).toBe('http://backend.internal/api/v1/track');
    expect(proxiedHeaders?.get('openpanel-client-id')).toBe('01924b12-0000-7000-8000-000000000001');
    expect(proxiedHeaders?.get('openpanel-client-ip')).toBe('203.0.113.195');
    expect(proxiedHeaders?.get('User-Agent')).toBe('Mozilla/5.0 TestBrowser');
    expect(JSON.parse(proxiedBody)).toEqual({ name: 'click_btn' });
  });

  it('proxies replay requests to /api/v1/replay', async () => {
    const handler = createRouteHandler({ apiUrl: 'http://backend.internal' });

    let proxiedUrl = '';
    globalThis.fetch = vi.fn().mockImplementation(async (url: string) => {
      proxiedUrl = url;
      return {
        status: 202,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ status: 'accepted' }),
      } as any;
    });

    const req = new Request('http://localhost:3000/api/analytics/replay', {
      method: 'POST',
      headers: {
        'openpanel-client-id': '01924b12-0000-7000-8000-000000000001',
      },
      body: JSON.stringify({ chunk_index: 0 }),
    });

    const response = await handler(req);
    expect(response.status).toBe(202);
    expect(proxiedUrl).toBe('http://backend.internal/api/v1/replay');
  });
});
