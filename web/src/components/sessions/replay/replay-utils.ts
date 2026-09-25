import type { IServiceEvent } from '@openpanel/db';

export function getEventOffsetMs(
  event: IServiceEvent,
  startTime: number,
): number {
  const t =
    typeof event.createdAt === 'object' && event.createdAt instanceof Date
      ? event.createdAt.getTime()
      : new Date(event.createdAt).getTime();
  return t - startTime;
}

/** Format a duration in milliseconds as M:SS */
export function formatDuration(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  const m = Math.floor(totalSeconds / 60);
  const s = totalSeconds % 60;
  return `${m}:${s.toString().padStart(2, '0')}`;
}

export function getRecordedOrigin(
  events: Array<{ type: number; data: unknown }>,
): string {
  const meta = events.find((e) => e.type === 4);
  if (
    meta &&
    typeof meta.data === 'object' &&
    meta.data !== null &&
    'href' in meta.data &&
    typeof (meta.data as { href: unknown }).href === 'string'
  ) {
    try {
      const url = new URL((meta.data as { href: string }).href);
      return url.origin;
    } catch {}
  }
  return '';
}

export function sanitizeNode(node: any, baseOrigin: string): void {
  if (!node || typeof node !== 'object') return;

  // 1. Fix nodes with _cssText:
  // rrweb's buildStyleNode checks:
  //   if (n2.childNodes.length) applyCssSplits(n2, cssText, hackCss, cache)
  //   else styleEl.appendChild(doc.createTextNode(cssText))
  // When browser extensions (e.g. Dark Reader) inject child elements inside a <link>
  // or <style>, childNodes.length is > 0 but childTextNodes is empty, so applyCssSplits
  // does nothing and skips appending _cssText! Setting childNodes = [] ensures rrweb
  // creates a text node containing the complete CSS rules.
  if (node.attributes && typeof node.attributes._cssText === 'string') {
    const textChildren = Array.isArray(node.childNodes)
      ? node.childNodes.filter(
          (c: any) =>
            c &&
            c.type === 3 &&
            typeof c.textContent === 'string' &&
            c.textContent.trim().length > 0,
        )
      : [];
    const elementChildren = Array.isArray(node.childNodes)
      ? node.childNodes.filter((c: any) => c && c.type === 2)
      : [];
    if (elementChildren.length > 0 || textChildren.length === 0) {
      node.childNodes = [];
    }
  }

  // 2. Rebase relative links/resources (e.g. stylesheets, fonts, images) to recorded origin
  // so the player iframe doesn't request them from localhost:3000 (dashboard host).
  if (baseOrigin && node.attributes && typeof node.attributes === 'object') {
    for (const attr of ['href', 'src']) {
      const val = node.attributes[attr];
      if (
        typeof val === 'string' &&
        (val.startsWith('/') || val.startsWith('./') || val.startsWith('../'))
      ) {
        try {
          node.attributes[attr] = new URL(val, baseOrigin).href;
        } catch {}
      }
    }
  }

  if (Array.isArray(node.childNodes)) {
    for (const child of node.childNodes) {
      sanitizeNode(child, baseOrigin);
    }
  }
}

export function sanitizeReplayEvents<
  T extends { type: number; data: unknown; timestamp?: number },
>(events: T[]): T[] {
  const baseOrigin = getRecordedOrigin(events);
  for (const e of events) {
    if (e.type === 2 && e.data && typeof e.data === 'object' && 'node' in e.data) {
      sanitizeNode((e.data as any).node, baseOrigin);
    } else if (
      e.type === 3 &&
      e.data &&
      typeof e.data === 'object' &&
      'adds' in e.data
    ) {
      const adds = (e.data as any).adds;
      if (Array.isArray(adds)) {
        for (const add of adds) {
          if (add && add.node) {
            sanitizeNode(add.node, baseOrigin);
          }
        }
      }
    }
  }
  return events;
}
