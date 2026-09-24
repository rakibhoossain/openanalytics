import type {
  IAliasPayload,
  IAssignGroupPayload,
  IDecrementPayload,
  IGroupPayload,
  IIdentifyPayload,
  IIncrementPayload,
  ITrackHandlerPayload,
  ITrackPayload,
} from '@openpanel/validation/src/track.validation';
import { Api } from './api';

export type AliasPayload = IAliasPayload;
export type AssignGroupPayload = IAssignGroupPayload;
export type DecrementPayload = IDecrementPayload;
export type GroupPayload = IGroupPayload;
export type IdentifyPayload = IIdentifyPayload;
export type IncrementPayload = IIncrementPayload;
export type TrackHandlerPayload = ITrackHandlerPayload;
export type TrackPayload = ITrackPayload;

export interface TrackProperties {
  [key: string]: unknown;
  profileId?: string;
  groups?: string[];
}

export type UpsertGroupPayload = GroupPayload;

export interface OpenAnalyticsOptions {
  clientId: string;
  clientSecret?: string;
  apiUrl?: string;
  sdk?: string;
  sdkVersion?: string;
  /** Queue events until identify() is called with a profileId */
  waitForProfile?: boolean;
  filter?: (payload: TrackHandlerPayload) => boolean;
  /** When true, events are queued until ready() is called */
  disabled?: boolean;
  debug?: boolean;
}

export class OpenAnalytics {
  api: Api;
  options: OpenAnalyticsOptions;
  profileId?: string | number;
  groups: string[] = [];
  deviceId?: string;
  sessionId?: string;
  global?: Record<string, unknown>;
  queue: TrackHandlerPayload[] = [];

  constructor(options: OpenAnalyticsOptions) {
    this.options = options;

    const defaultHeaders: Record<string, string> = {
      'openpanel-client-id': options.clientId,
      'X-Shop-Id': options.clientId,
      'openanalytics-client-id': options.clientId,
    };

    if (options.clientSecret) {
      defaultHeaders['openpanel-client-secret'] = options.clientSecret;
      defaultHeaders['X-Client-Secret'] = options.clientSecret;
    }

    defaultHeaders['openpanel-sdk-name'] = options.sdk || 'node';
    defaultHeaders['openpanel-sdk-version'] =
      options.sdkVersion || '1.0.0';

    this.api = new Api({
      baseUrl: options.apiUrl || 'http://localhost:8080',
      defaultHeaders,
    });
  }

  init() {
    // placeholder for future hooks
  }

  ready() {
    this.options.disabled = false;
    this.options.waitForProfile = false;
    this.flush();
  }

  private shouldQueue(payload: TrackHandlerPayload): boolean {
    if (this.options.disabled) {
      return true;
    }
    if (this.options.waitForProfile && !this.profileId) {
      return true;
    }
    if (payload.type === 'replay' && !this.sessionId) {
      return true;
    }
    return false;
  }

  addQueue(payload: TrackHandlerPayload) {
    if (payload.type === 'track') {
      payload.payload.properties = {
        ...(payload.payload.properties ?? {}),
        __timestamp: new Date().toISOString(),
      };
    }

    this.queue.push(payload);
  }

  async send(payload: TrackHandlerPayload) {
    if (this.options.filter && !this.options.filter(payload)) {
      return Promise.resolve();
    }

    if (this.shouldQueue(payload)) {
      this.addQueue(payload);
      return Promise.resolve();
    }

    // Disable keepalive for replay since large snapshot blobs break browser 64KB keepalive limit
    const endpoint = payload.type === 'replay' ? '/api/v1/replay' : '/api/v1/track';
    const result = await this.api.fetch<
      any,
      { deviceId?: string; sessionId?: string; session_id?: string; event_id?: string }
    >(endpoint, payload.type === 'replay' ? payload.payload : payload, { keepalive: payload.type !== 'replay' });

    if (result?.deviceId) {
      this.deviceId = result.deviceId;
    }
    const returnedSession = result?.sessionId || result?.session_id;
    const hadSession = !!this.sessionId;
    if (returnedSession) {
      this.sessionId = returnedSession;
    }

    // Flush queued items (such as buffered replay chunks) when sessionId arrives
    if (!hadSession && this.sessionId) {
      this.flush();
    }

    return result;
  }

  setGlobalProperties(properties: Record<string, unknown>) {
    this.global = {
      ...this.global,
      ...properties,
    };
  }

  track(name: string, properties?: TrackProperties) {
    this.log('track event', name, properties);
    const { groups: groupsOverride, profileId, ...rest } = properties ?? {};
    const mergedGroups = [
      ...new Set([...this.groups, ...(groupsOverride ?? [])]),
    ];
    return this.send({
      type: 'track',
      payload: {
        name,
        profileId: profileId ?? this.profileId,
        groups: mergedGroups.length > 0 ? mergedGroups : undefined,
        properties: {
          ...(this.global ?? {}),
          ...rest,
        },
      },
    });
  }

  identify(payload: IdentifyPayload) {
    this.log('identify user', payload);
    if (payload.profileId) {
      this.profileId = payload.profileId;
      this.flush();
    }

    if (payload.profileId && Object.keys(payload).length > 1) {
      return this.send({
        type: 'identify',
        payload: {
          ...payload,
          properties: {
            ...this.global,
            ...payload.properties,
          },
        },
      });
    }
  }

  upsertGroup(payload: UpsertGroupPayload) {
    this.log('upsert group', payload);
    return this.send({
      type: 'group',
      payload,
    });
  }

  setGroup(groupId: string) {
    this.log('set group', groupId);
    if (!this.groups.includes(groupId)) {
      this.groups = [...this.groups, groupId];
    }
    return this.send({
      type: 'assign_group',
      payload: {
        groupIds: [groupId],
        profileId: this.profileId,
      },
    });
  }

  setGroups(groupIds: string[]) {
    this.log('set groups', groupIds);
    this.groups = [...new Set([...this.groups, ...groupIds])];
    return this.send({
      type: 'assign_group',
      payload: {
        groupIds,
        profileId: this.profileId,
      },
    });
  }

  alias(_payload: AliasPayload) {
    // noop
  }

  increment(payload: IncrementPayload) {
    return this.send({
      type: 'increment',
      payload,
    });
  }

  decrement(payload: DecrementPayload) {
    return this.send({
      type: 'decrement',
      payload,
    });
  }

  revenue(
    amount: number,
    properties?: TrackProperties & { deviceId?: string }
  ) {
    const deviceId = properties?.deviceId;
    delete properties?.deviceId;
    return this.track('revenue', {
      ...(properties ?? {}),
      ...(deviceId ? { __deviceId: deviceId } : {}),
      __revenue: amount,
    });
  }

  getDeviceId(): string {
    return this.deviceId ?? '';
  }

  getSessionId(): string {
    return this.sessionId ?? '';
  }

  clear() {
    this.profileId = undefined;
    this.groups = [];
    this.deviceId = undefined;
    this.sessionId = undefined;
  }

  private buildFlushPayload(
    item: TrackHandlerPayload
  ): TrackHandlerPayload['payload'] {
    if (item.type === 'replay') {
      return {
        ...item.payload,
        sessionId: this.sessionId,
      };
    }
    if (item.type === 'track') {
      const queuedGroups =
        'groups' in item.payload ? (item.payload.groups ?? []) : [];
      const mergedGroups = [...new Set([...this.groups, ...queuedGroups])];
      return {
        ...item.payload,
        profileId: item.payload.profileId ?? this.profileId,
        groups: mergedGroups.length > 0 ? mergedGroups : undefined,
      };
    }
    if (
      item.type === 'identify' ||
      item.type === 'increment' ||
      item.type === 'decrement'
    ) {
      return {
        ...item.payload,
        profileId: item.payload.profileId ?? this.profileId,
      } as TrackHandlerPayload['payload'];
    }
    if (item.type === 'assign_group') {
      return {
        ...item.payload,
        profileId: item.payload.profileId ?? this.profileId,
      };
    }
    return item.payload;
  }

  flush() {
    const remaining: TrackHandlerPayload[] = [];
    for (const item of this.queue) {
      if (this.shouldQueue(item)) {
        remaining.push(item);
        continue;
      }
      const payload = this.buildFlushPayload(item);
      this.send({ ...item, payload } as TrackHandlerPayload);
    }
    this.queue = remaining;
  }

  log(...args: any[]) {
    if (this.options.debug) {
      console.log('[OpenAnalytics]', ...args);
    }
  }
}
