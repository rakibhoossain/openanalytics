import { describe, expect, it, vi, beforeEach } from 'vitest';
import { OpenAnalytics } from './index';

describe('OpenAnalytics Core SDK', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('queues events when disabled: true until ready() is called', async () => {
    const oa = new OpenAnalytics({
      clientId: '01924b12-0000-7000-8000-000000000001',
      disabled: true,
    });

    const sendSpy = vi.spyOn(oa.api, 'fetch').mockResolvedValue({
      deviceId: 'dev_123',
      sessionId: '01924b12-0000-7000-8000-000000000099',
    });

    oa.track('test_event', { key: 'value' });
    expect(oa.queue.length).toBe(1);
    expect(sendSpy).not.toHaveBeenCalled();

    oa.ready();
    expect(sendSpy).toHaveBeenCalledTimes(1);
    expect(oa.queue.length).toBe(0);
  });

  it('buffers replay chunks until sessionId is received from server', async () => {
    const oa = new OpenAnalytics({
      clientId: '01924b12-0000-7000-8000-000000000001',
    });

    let fetchCount = 0;
    vi.spyOn(oa.api, 'fetch').mockImplementation(async (path: string) => {
      fetchCount++;
      if (path === '/api/v1/track') {
        return {
          deviceId: 'dev_123',
          sessionId: '01924b12-0000-7000-8000-000000000099',
        };
      }
      return { status: 'accepted' };
    });

    // Send replay chunk before any session exists
    await oa.send({
      type: 'replay',
      payload: {
        chunk_index: 0,
        events_count: 5,
        is_full_snapshot: true,
        started_at: '2026-09-24T15:00:00Z',
        ended_at: '2026-09-24T15:00:10Z',
        payload: '[]',
      },
    });

    // Should be queued in memory waiting for sessionId
    expect(oa.queue.length).toBe(1);

    // Now send normal track event — returns sessionId and triggers auto-flush of replay queue!
    await oa.track('page_view', { path: '/' });

    expect(oa.sessionId).toBe('01924b12-0000-7000-8000-000000000099');
    expect(oa.queue.length).toBe(0);
    expect(fetchCount).toBe(2); // 1 for track, 1 for flushed replay chunk
  });

  it('sets correct headers on outgoing API requests', () => {
    const oa = new OpenAnalytics({
      clientId: '01924b12-0000-7000-8000-000000000001',
      clientSecret: 'secret_123',
      apiUrl: 'http://localhost:8080',
    });

    expect((oa.api as any).headers['openpanel-client-id']).toBe(
      '01924b12-0000-7000-8000-000000000001'
    );
    expect((oa.api as any).headers['X-Shop-Id']).toBe(
      '01924b12-0000-7000-8000-000000000001'
    );
    expect((oa.api as any).headers['X-Client-Secret']).toBe('secret_123');
  });
});
