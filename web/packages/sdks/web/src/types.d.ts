import type { TrackProperties } from '@openanalytics/sdk';
import type { OpenAnalyticsWeb, OpenAnalyticsWebOptions } from './index';

type ExposedMethodsNames =
  | 'track'
  | 'identify'
  | 'setGlobalProperties'
  | 'increment'
  | 'decrement'
  | 'clear'
  | 'revenue'
  | 'flushRevenue'
  | 'clearRevenue'
  | 'pendingRevenue'
  | 'screenView'
  | 'getDeviceId'
  | 'getSessionId';

export type ExposedMethods = {
  [K in ExposedMethodsNames]: OpenAnalyticsWeb[K] extends (...args: any[]) => any
    ? [K, ...Parameters<OpenAnalyticsWeb[K]>]
    : never;
}[ExposedMethodsNames];

export type OpenAnalyticsMethodNames = ExposedMethodsNames | 'init';
export type OpenAnalyticsMethods =
  | ExposedMethods
  | ['init', OpenAnalyticsWebOptions]
  | [
      'screenView',
      string | TrackProperties | undefined,
      TrackProperties | undefined,
    ];

type OpenAnalyticsMethodSignatures = {
  [K in ExposedMethodsNames]: OpenAnalyticsWeb[K];
} & {
  screenView(
    pathOrProperties?: string | TrackProperties,
    properties?: TrackProperties
  ): void;
};

type OpenAnalyticsAPI = OpenAnalyticsMethodSignatures & {
  q?: OpenAnalyticsMethods[];
  (...args: OpenAnalyticsMethods): void;
};

declare global {
  interface Window {
    openanalytics?: OpenAnalyticsWeb;
    oa: OpenAnalyticsAPI;
  }
}
