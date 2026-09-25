'use client';

import type {
  DecrementPayload,
  IdentifyPayload,
  IncrementPayload,
  OpenAnalyticsMethodNames,
  OpenAnalyticsWebOptions,
  TrackProperties,
} from '@openanalytics/web';
import { OpenAnalyticsWeb } from '@openanalytics/web';
import React, { useEffect, useRef } from 'react';

export * from '@openanalytics/web';

declare const window: any;

if (typeof window !== 'undefined' && !window.oa) {
  const q: any[] = [];
  const stub = function (...args: any[]) {
    q.push(args);
  };
  stub.q = q;
  window.oa = stub;
}

export type OpenAnalyticsComponentProps = Omit<OpenAnalyticsWebOptions, 'filter'> & {
  profileId?: string;
  scriptUrl?: string;
  filter?: (payload: any) => boolean;
  globalProperties?: Record<string, unknown>;
  strategy?: 'beforeInteractive' | 'afterInteractive' | 'lazyOnload' | 'worker';
};

export function OpenAnalyticsComponent({
  profileId,
  globalProperties,
  strategy: _strategy,
  scriptUrl: _scriptUrl,
  ...options
}: OpenAnalyticsComponentProps) {
  const initializedRef = useRef(false);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    if (initializedRef.current) return;
    initializedRef.current = true;

    try {
      const oa = new OpenAnalyticsWeb({
        ...options,
        sdk: 'nextjs',
        sdkVersion: '1.0.0',
      });

      if (profileId) {
        oa.identify({ profileId });
      }
      if (globalProperties) {
        oa.setGlobalProperties(globalProperties);
      }

      // Create a Proxy that supports both window.oa('track', ...) and window.oa.track(...)
      const oaCallable = new Proxy(
        ((method: string, ...args: any[]) => {
          const fn = (oa as any)[method]
            ? (oa as any)[method].bind(oa)
            : undefined;
          if (typeof fn === 'function') {
            return fn(...args);
          } else {
            console.warn(`[OpenAnalytics] ${method} is not a function`);
          }
        }) as typeof oa & ((method: string, ...args: any[]) => any),
        {
          get(target, prop) {
            if (prop === 'q') return undefined;
            const value = (oa as any)[prop];
            if (typeof value === 'function') {
              return value.bind(oa);
            }
            return value;
          },
        }
      );

      // Drain any queued calls if previous stub was initialized
      if (window.oa && Array.isArray(window.oa.q)) {
        window.oa.q.forEach((item: any[]) => {
          if (item && item.length > 0 && item[0] !== 'init') {
            (oaCallable as any)(item[0], ...item.slice(1));
          }
        });
      }

      window.oa = oaCallable;
      window.openanalytics = oa;
    } catch (err) {
      console.error('[OpenAnalytics] Failed to initialize web tracker:', err);
    }
  }, []);

  return null;
}

// Alias for convenience
export const OpenAnalytics = OpenAnalyticsComponent;

export function IdentifyComponent(props: IdentifyPayload) {
  useEffect(() => {
    if (typeof window !== 'undefined' && window.oa) {
      if (typeof window.oa.identify === 'function') {
        window.oa.identify(props);
      } else if (typeof window.oa === 'function') {
        window.oa('identify', props);
      }
    }
  }, [props]);
  return null;
}

export function SetGlobalPropertiesComponent(props: Record<string, unknown>) {
  useEffect(() => {
    if (typeof window !== 'undefined' && window.oa) {
      if (typeof window.oa.setGlobalProperties === 'function') {
        window.oa.setGlobalProperties(props);
      } else if (typeof window.oa === 'function') {
        window.oa('setGlobalProperties', props);
      }
    }
  }, [props]);
  return null;
}

export function useOpenAnalytics() {
  return {
    track,
    screenView,
    identify,
    increment,
    decrement,
    clear,
    setGlobalProperties,
    revenue,
    flushRevenue,
    clearRevenue,
    pendingRevenue,
    getDeviceId,
    getSessionId,
  };
}

// Backwards compatibility alias
export const useOpenPanel = useOpenAnalytics;

function setGlobalProperties(properties: Record<string, unknown>) {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.setGlobalProperties === 'function') {
    window.oa.setGlobalProperties(properties);
  } else if (typeof window.oa === 'function') {
    window.oa('setGlobalProperties', properties);
  }
}

function track(name: string, properties?: TrackProperties) {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.track === 'function') {
    window.oa.track(name, properties);
  } else if (typeof window.oa === 'function') {
    window.oa('track', name, properties);
  }
}

function screenView(properties?: TrackProperties): void;
function screenView(path: string, properties?: TrackProperties): void;
function screenView(
  pathOrProperties?: string | TrackProperties,
  propertiesOrUndefined?: TrackProperties
) {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.screenView === 'function') {
    window.oa.screenView(pathOrProperties, propertiesOrUndefined);
  } else if (typeof window.oa === 'function') {
    window.oa('screenView', pathOrProperties, propertiesOrUndefined);
  }
}

function identify(payload: IdentifyPayload) {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.identify === 'function') {
    window.oa.identify(payload);
  } else if (typeof window.oa === 'function') {
    window.oa('identify', payload);
  }
}

function increment(payload: IncrementPayload) {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.increment === 'function') {
    window.oa.increment(payload);
  } else if (typeof window.oa === 'function') {
    window.oa('increment', payload);
  }
}

function decrement(payload: DecrementPayload) {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.decrement === 'function') {
    window.oa.decrement(payload);
  } else if (typeof window.oa === 'function') {
    window.oa('decrement', payload);
  }
}

function getDeviceId(): string {
  if (typeof window === 'undefined' || !window.oa) return '';
  if (typeof window.oa.getDeviceId === 'function') {
    return window.oa.getDeviceId();
  }
  return window.oa.deviceId || '';
}

function getSessionId(): string {
  if (typeof window === 'undefined' || !window.oa) return '';
  if (typeof window.oa.getSessionId === 'function') {
    return window.oa.getSessionId();
  }
  return window.oa.sessionId || '';
}

function clearRevenue() {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.clearRevenue === 'function') {
    window.oa.clearRevenue();
  } else if (typeof window.oa === 'function') {
    window.oa('clearRevenue');
  }
}

function pendingRevenue(amount: number, properties?: Record<string, unknown>) {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.pendingRevenue === 'function') {
    window.oa.pendingRevenue(amount, properties);
  } else if (typeof window.oa === 'function') {
    window.oa('pendingRevenue', amount, properties);
  }
}

function revenue(amount: number, properties?: Record<string, unknown>) {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.revenue === 'function') {
    return window.oa.revenue(amount, properties);
  } else if (typeof window.oa === 'function') {
    return window.oa('revenue', amount, properties);
  }
}

function flushRevenue() {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.flushRevenue === 'function') {
    return window.oa.flushRevenue();
  } else if (typeof window.oa === 'function') {
    return window.oa('flushRevenue');
  }
}

function clear() {
  if (typeof window === 'undefined' || !window.oa) return;
  if (typeof window.oa.clear === 'function') {
    window.oa.clear();
  } else if (typeof window.oa === 'function') {
    window.oa('clear');
  }
}
