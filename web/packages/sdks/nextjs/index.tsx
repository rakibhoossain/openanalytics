import type {
  DecrementPayload,
  IdentifyPayload,
  IncrementPayload,
  OpenAnalyticsMethodNames,
  OpenAnalyticsWebOptions,
  TrackProperties,
} from '@openanalytics/web';
import { getInitSnippet } from '@openanalytics/web';
import Script from 'next/script.js';
import React from 'react';

export * from '@openanalytics/web';

declare const window: any;

const DEFAULT_SCRIPT_URL = '/oa.js';

export type OpenAnalyticsComponentProps = Omit<OpenAnalyticsWebOptions, 'filter'> & {
  profileId?: string;
  scriptUrl?: string;
  filter?: string;
  globalProperties?: Record<string, unknown>;
  strategy?: 'beforeInteractive' | 'afterInteractive' | 'lazyOnload' | 'worker';
};

const stringify = (obj: unknown) => {
  if (typeof obj === 'object' && obj !== null && obj !== undefined) {
    const entries = Object.entries(obj).map(([key, value]) => {
      if (key === 'filter') {
        return `"${key}":${value}`;
      }
      return `"${key}":${JSON.stringify(value)}`;
    });
    return `{${entries.join(',')}}`;
  }

  return JSON.stringify(obj);
};

export function OpenAnalyticsComponent({
  profileId,
  scriptUrl,
  globalProperties,
  strategy = 'afterInteractive',
  ...options
}: OpenAnalyticsComponentProps) {
  const methods: { name: OpenAnalyticsMethodNames; value: unknown }[] = [
    {
      name: 'init',
      value: {
        ...options,
        sdk: 'nextjs',
        sdkVersion: '1.0.0',
      },
    },
  ];
  if (profileId) {
    methods.push({
      name: 'identify',
      value: {
        profileId,
      },
    });
  }
  if (globalProperties) {
    methods.push({
      name: 'setGlobalProperties',
      value: globalProperties,
    });
  }

  return (
    <>
      <Script async defer src={scriptUrl || DEFAULT_SCRIPT_URL} />
      <Script
        dangerouslySetInnerHTML={{
          __html: `${getInitSnippet()}
          ${methods
            .map((method) => {
              return `window.oa('${method.name}', ${stringify(method.value)});`;
            })
            .join('\n')}`,
        }}
        id="openanalytics-init"
        strategy={strategy}
      />
    </>
  );
}

// Alias for convenience
export const OpenAnalytics = OpenAnalyticsComponent;

export function IdentifyComponent(props: IdentifyPayload) {
  return (
    <Script
      dangerouslySetInnerHTML={{
        __html: `window.oa('identify', ${JSON.stringify(props)});`,
      }}
    />
  );
}

export function SetGlobalPropertiesComponent(props: Record<string, unknown>) {
  return (
    <Script
      dangerouslySetInnerHTML={{
        __html: `window.oa('setGlobalProperties', ${JSON.stringify(props)});`,
      }}
    />
  );
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
  window.oa?.('setGlobalProperties', properties);
}

function track(name: string, properties?: TrackProperties) {
  window.oa?.('track', name, properties);
}

function screenView(properties?: TrackProperties): void;
function screenView(path: string, properties?: TrackProperties): void;
function screenView(
  pathOrProperties?: string | TrackProperties,
  propertiesOrUndefined?: TrackProperties
) {
  window.oa?.('screenView', pathOrProperties, propertiesOrUndefined);
}

function identify(payload: IdentifyPayload) {
  window.oa?.('identify', payload);
}

function increment(payload: IncrementPayload) {
  window.oa?.('increment', payload);
}

function decrement(payload: DecrementPayload) {
  window.oa?.('decrement', payload);
}

function getDeviceId(): string {
  return window.oa?.getDeviceId?.() ?? '';
}

function getSessionId(): string {
  return window.oa?.getSessionId?.() ?? '';
}

function clearRevenue() {
  window.oa?.clearRevenue?.();
}

function pendingRevenue(amount: number, properties?: Record<string, unknown>) {
  window.oa?.pendingRevenue?.(amount, properties);
}

function revenue(amount: number, properties?: Record<string, unknown>) {
  return window.oa?.revenue?.(amount, properties);
}

function flushRevenue() {
  return window.oa?.flushRevenue?.();
}

function clear() {
  window.oa?.('clear');
}
