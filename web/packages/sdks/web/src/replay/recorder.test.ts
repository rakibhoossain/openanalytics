// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { startReplayRecorder, stopReplayRecorder, type ReplayChunkPayload } from './recorder';

vi.mock('rrweb', () => {
  let emitFn: any = null;
  return {
    record: vi.fn(({ emit }: any) => {
      emitFn = emit;
      return () => {
        emitFn = null;
      };
    }),
    __triggerEmit: (event: any, isCheckout?: boolean) => {
      if (emitFn) emitFn(event, isCheckout);
    },
  };
});

describe('Session Replay Recorder', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    stopReplayRecorder();
    vi.useRealTimers();
  });

  it('chunks and flushes when buffer reaches maxEventsPerChunk', async () => {
    const { __triggerEmit } = await import('rrweb') as any;
    const sentChunks: ReplayChunkPayload[] = [];

    startReplayRecorder(
      {
        maxEventsPerChunk: 3,
        flushIntervalMs: 60_000,
      },
      (chunk) => sentChunks.push(chunk)
    );

    __triggerEmit({ type: 4, timestamp: 1000, data: { href: 'http://a' } });
    __triggerEmit({ type: 3, timestamp: 2000, data: { source: 1 } });
    expect(sentChunks.length).toBe(0);

    // 3rd event reaches limit (3) -> triggers flush!
    __triggerEmit({ type: 3, timestamp: 3000, data: { source: 2 } });
    expect(sentChunks.length).toBe(1);
    expect(sentChunks[0]?.chunk_index).toBe(0);
    expect(sentChunks[0]?.events_count).toBe(3);
  });

  it('recursively bisects payloads that exceed maxPayloadBytes', async () => {
    const { __triggerEmit } = await import('rrweb') as any;
    const sentChunks: ReplayChunkPayload[] = [];

    // Set tiny byte ceiling to force bisection
    startReplayRecorder(
      {
        maxEventsPerChunk: 10,
        flushIntervalMs: 1000,
        maxPayloadBytes: 150, // Tiny byte limit
      },
      (chunk) => sentChunks.push(chunk)
    );

    // Emit 4 large events
    __triggerEmit({ type: 4, timestamp: 1000, data: { text: 'large string payload 123456789' } });
    __triggerEmit({ type: 3, timestamp: 2000, data: { text: 'large string payload 123456789' } });
    __triggerEmit({ type: 3, timestamp: 3000, data: { text: 'large string payload 123456789' } });
    __triggerEmit({ type: 3, timestamp: 4000, data: { text: 'large string payload 123456789' } });

    // Trigger timer flush
    vi.advanceTimersByTime(1000);

    // Should have bisected the 4 events into sub-150-byte chunks!
    expect(sentChunks.length).toBeGreaterThan(1);
    for (const chunk of sentChunks) {
      expect(new TextEncoder().encode(chunk.payload).length).toBeLessThanOrEqual(150);
    }
  });

  it('marks is_full_snapshot true when full snapshot (type 2) is present', async () => {
    const { __triggerEmit } = await import('rrweb') as any;
    const sentChunks: ReplayChunkPayload[] = [];

    startReplayRecorder(
      {
        maxEventsPerChunk: 2,
        flushIntervalMs: 60_000,
      },
      (chunk) => sentChunks.push(chunk)
    );

    __triggerEmit({ type: 2, timestamp: 1000, data: { node: {} } }); // Type 2 = FullSnapshot
    __triggerEmit({ type: 3, timestamp: 2000, data: { source: 1 } });

    expect(sentChunks.length).toBe(1);
    expect(sentChunks[0]?.is_full_snapshot).toBe(true);
  });
});
