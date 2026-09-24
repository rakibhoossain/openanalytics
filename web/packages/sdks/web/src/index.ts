import type {
  OpenAnalyticsOptions as OpenAnalyticsBaseOptions,
  TrackProperties,
} from '@openanalytics/sdk';
import { OpenAnalytics as OpenAnalyticsBase } from '@openanalytics/sdk';

export type * from '@openanalytics/sdk';
export { OpenAnalytics as OpenAnalyticsBase } from '@openanalytics/sdk';
export { getInitSnippet } from './init-snippet';
export type {
  OpenAnalyticsMethodNames,
  OpenAnalyticsMethods,
  ExposedMethods,
} from './types';

export type SessionReplayOptions = {
  enabled: boolean;
  sampleRate?: number;
  maskAllInputs?: boolean;
  maskAllText?: boolean;
  unmaskTextSelector?: string;
  blockSelector?: string;
  blockClass?: string;
  ignoreSelector?: string;
  flushIntervalMs?: number;
  maxEventsPerChunk?: number;
  maxPayloadBytes?: number;
  scriptUrl?: string;
};

declare const __OPENANALYTICS_REPLAY_URL__: string | undefined;

const _replayScriptRef: HTMLScriptElement | null =
  typeof document !== 'undefined'
    ? (document.currentScript as HTMLScriptElement | null)
    : null;

export type OpenAnalyticsWebOptions = OpenAnalyticsBaseOptions & {
  trackOutgoingLinks?: boolean;
  trackScreenViews?: boolean;
  trackAttributes?: boolean;
  trackHashChanges?: boolean;
  sessionReplay?: SessionReplayOptions;
};

function toCamelCase(str: string) {
  return str.replace(/([-_][a-z])/gi, ($1) =>
    $1.toUpperCase().replace('-', '').replace('_', '')
  );
}

type PendingRevenue = {
  amount: number;
  properties?: Record<string, unknown>;
};

export class OpenAnalyticsWeb extends OpenAnalyticsBase {
  private lastPath = '';
  private debounceTimer: any;
  private pendingRevenues: PendingRevenue[] = [];

  constructor(public webOptions: OpenAnalyticsWebOptions = { clientId: '' }) {
    super({
      sdk: 'web',
      sdkVersion: '1.0.0',
      ...webOptions,
    });

    if (!this.isServer()) {
      try {
        const pending = sessionStorage.getItem('oa-pending-revenues');
        if (pending) {
          const parsed = JSON.parse(pending);
          if (Array.isArray(parsed)) {
            this.pendingRevenues = parsed;
          }
        }
      } catch {
        this.pendingRevenues = [];
      }

      this.setGlobalProperties({
        __referrer: document.referrer,
      });

      if (this.webOptions.trackScreenViews) {
        this.trackScreenViews();
        setTimeout(() => this.screenView(), 0);
      }

      if (this.webOptions.trackOutgoingLinks) {
        this.trackOutgoingLinks();
      }

      if (this.webOptions.trackAttributes) {
        this.trackAttributes();
      }

      if (this.webOptions.sessionReplay?.enabled) {
        const sampleRate = this.webOptions.sessionReplay.sampleRate ?? 1;
        const sampled = Math.random() < sampleRate;
        if (sampled) {
          this.loadReplayModule().then((mod) => {
            if (!mod) {
              return;
            }
            mod.startReplayRecorder(this.webOptions.sessionReplay!, (chunk) => {
              this.send({
                type: 'replay',
                payload: {
                  ...chunk,
                  sessionId: this.sessionId,
                },
              });
            });
          });
        }
      }
    }
  }

  private async loadReplayModule(): Promise<typeof import('./replay') | null> {
    try {
      if (typeof __OPENANALYTICS_REPLAY_URL__ !== 'undefined') {
        const scriptEl = _replayScriptRef;
        const url =
          this.webOptions.sessionReplay?.scriptUrl ||
          scriptEl?.src?.replace('.js', '-replay.js') ||
          '/oa-replay.js';

        if ((window as any).__openanalytics_replay) {
          return (window as any).__openanalytics_replay;
        }

        return new Promise((resolve) => {
          const script = document.createElement('script');
          script.src = url;
          script.onload = () => {
            resolve((window as any).__openanalytics_replay ?? null);
          };
          script.onerror = () => {
            console.warn('[OpenAnalytics] Failed to load replay script from', url);
            resolve(null);
          };
          document.head.appendChild(script);
        });
      }
      return await import('./replay');
    } catch (e) {
      console.warn('[OpenAnalytics] Failed to load replay module', e);
      return null;
    }
  }

  private debounce(func: () => void, delay: number) {
    clearTimeout(this.debounceTimer);
    this.debounceTimer = setTimeout(func, delay);
  }

  private isServer() {
    return typeof document === 'undefined';
  }

  public trackOutgoingLinks() {
    if (this.isServer()) {
      return;
    }

    document.addEventListener('click', (event) => {
      const target = event.target as HTMLElement;
      const link = target.closest('a');
      if (link && target) {
        const href = link.getAttribute('href');
        if (href?.startsWith('http')) {
          try {
            const linkUrl = new URL(href);
            const currentHostname = window.location.hostname;
            if (linkUrl.hostname !== currentHostname) {
              super.track('link_out', {
                href,
                text:
                  link.innerText ||
                  link.getAttribute('title') ||
                  target.getAttribute('alt') ||
                  target.getAttribute('title'),
              });
            }
          } catch {
            // Invalid URL, skip
          }
        }
      }
    });
  }

  public trackScreenViews() {
    if (this.isServer()) {
      return;
    }

    const oldPushState = history.pushState;
    history.pushState = function pushState(...args) {
      const ret = oldPushState.apply(this, args);
      window.dispatchEvent(new Event('pushstate'));
      window.dispatchEvent(new Event('locationchange'));
      return ret;
    };

    const oldReplaceState = history.replaceState;
    history.replaceState = function replaceState(...args) {
      const ret = oldReplaceState.apply(this, args);
      window.dispatchEvent(new Event('replacestate'));
      window.dispatchEvent(new Event('locationchange'));
      return ret;
    };

    window.addEventListener('popstate', () => {
      window.dispatchEvent(new Event('locationchange'));
    });

    const eventHandler = () => this.debounce(() => this.screenView(), 50);

    if (this.webOptions.trackHashChanges) {
      window.addEventListener('hashchange', eventHandler);
    } else {
      window.addEventListener('locationchange', eventHandler);
    }
  }

  public trackAttributes() {
    if (this.isServer()) {
      return;
    }

    document.addEventListener('click', (event) => {
      const target = event.target as HTMLElement;
      const btn = target.closest('button');
      const anchor = target.closest('a');
      const element = btn?.getAttribute('data-track')
        ? btn
        : anchor?.getAttribute('data-track')
          ? anchor
          : null;
      if (element) {
        const properties: Record<string, unknown> = {};
        for (const attr of Array.from(element.attributes)) {
          if (attr.name.startsWith('data-') && attr.name !== 'data-track') {
            properties[toCamelCase(attr.name.replace(/^data-/, ''))] =
              attr.value;
          }
        }
        const name = element.getAttribute('data-track');
        if (name) {
          super.track(name, properties);
        }
      }
    });
  }

  track(name: string, properties?: TrackProperties) {
    return super.track(name, { ...properties, __path: this.lastPath });
  }

  screenView(properties?: TrackProperties): void;
  screenView(path: string, properties?: TrackProperties): void;
  screenView(
    pathOrProperties?: string | TrackProperties,
    propertiesOrUndefined?: TrackProperties
  ): void {
    if (this.isServer()) {
      return;
    }

    let path: string;
    let properties: TrackProperties | undefined;

    if (typeof pathOrProperties === 'string') {
      path = pathOrProperties;
      properties = propertiesOrUndefined;
    } else {
      path = window.location.href;
      properties = pathOrProperties;
    }

    if (this.lastPath === path) {
      return;
    }

    this.lastPath = path;
    super.track('screen_view', {
      ...(properties ?? {}),
      __path: path,
      __title: document.title,
    });
  }

  async flushRevenue() {
    const promises = this.pendingRevenues.map((pending) =>
      super.revenue(pending.amount, pending.properties)
    );
    await Promise.all(promises);
    this.clearRevenue();
  }

  clearRevenue() {
    this.pendingRevenues = [];
    if (!this.isServer()) {
      try {
        sessionStorage.removeItem('oa-pending-revenues');
      } catch {}
    }
  }

  pendingRevenue(amount: number, properties?: Record<string, unknown>) {
    this.pendingRevenues.push({ amount, properties });
    if (!this.isServer()) {
      try {
        sessionStorage.setItem(
          'oa-pending-revenues',
          JSON.stringify(this.pendingRevenues)
        );
      } catch {}
    }
  }
}
