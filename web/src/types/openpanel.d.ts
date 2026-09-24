declare module '@openpanel/db' {
  export interface IServiceProfile {
    id: string;
    email: string;
    avatar: string;
    firstName: string;
    lastName: string;
    createdAt: Date | string;
    lastSeenAt: Date | string;
    isExternal: boolean;
    projectId?: string;
    groups?: string[];
    properties: Record<string, unknown> & {
      region?: string;
      country?: string;
      city?: string;
      os?: string;
      os_version?: string;
      browser?: string;
      browser_version?: string;
      referrer_name?: string;
      referrer_type?: string;
      device?: string;
      brand?: string;
      model?: string;
      referrer?: string;
    };
  }

  export interface EventMeta {
    id?: string;
    name: string;
    color?: string;
    conversion?: boolean;
    revenue?: boolean;
    target?: number;
    description?: string;
  }

  export interface IServiceEvent {
    id: string;
    name: string;
    deviceId: string;
    profileId: string;
    projectId: string;
    sessionId: string;
    properties: Record<string, unknown> & {
      hash?: string;
      query?: Record<string, unknown>;
    };
    createdAt: Date | string;
    country?: string | undefined;
    city?: string | undefined;
    region?: string | undefined;
    longitude?: number | undefined | null;
    latitude?: number | undefined | null;
    os?: string | undefined;
    osVersion?: string | undefined;
    browser?: string | undefined;
    browserVersion?: string | undefined;
    device?: string | undefined;
    brand?: string | undefined;
    model?: string | undefined;
    duration?: number;
    path: string;
    origin: string;
    referrer?: string | undefined;
    referrerName?: string | undefined;
    referrerType?: string | undefined;
    importedAt?: Date | undefined;
    profile?: IServiceProfile | undefined;
    meta?: EventMeta | undefined;
    sdkName?: string | undefined;
    sdkVersion?: string | undefined;
    revenue?: number;
    groups?: string[];
  }

  export interface IServiceEventMinimal {
    id: string;
    name: string;
    projectId: string;
    sessionId: string;
    createdAt: Date | string;
    country?: string | undefined;
    longitude?: number | undefined | null;
    latitude?: number | undefined | null;
    os?: string | undefined;
    browser?: string | undefined;
    device?: string | undefined;
    path: string;
    origin?: string;
    meta?: EventMeta | undefined;
    profile?: IServiceProfile | undefined;
    minimal?: boolean;
  }

  export interface IClickhouseEvent {
    id: string;
    name: string;
    device_id: string;
    profile_id: string;
    project_id: string;
    session_id: string;
    path: string;
    origin: string;
    referrer: string;
    referrer_name: string;
    referrer_type: string;
    duration: number;
    properties: Record<string, unknown>;
    created_at: string;
    country: string;
    city: string;
    region: string;
    longitude: number | null;
    latitude: number | null;
    os: string;
    os_version: string;
    browser: string;
    browser_version: string;
    device: string;
    brand: string;
    model: string;
    imported_at: string | null;
    sdk_name: string;
    sdk_version: string;
    revenue?: number;
    groups: string[];
    profile?: IServiceProfile;
    meta?: EventMeta;
  }

  export interface IServiceSession {
    id: string;
    profileId: string;
    eventCount: number;
    screenViewCount: number;
    entryPath: string;
    entryOrigin: string;
    exitPath: string;
    exitOrigin: string;
    createdAt: Date | string;
    endedAt: Date | string;
    referrer: string;
    referrerName: string;
    referrerType: string;
    os: string;
    osVersion: string;
    browser: string;
    browserVersion: string;
    device: string;
    brand: string;
    model: string;
    country: string;
    region: string;
    city: string;
    longitude: number | null;
    latitude: number | null;
    isBounce: boolean;
    projectId: string;
    deviceId: string;
    duration: number;
    utmMedium: string;
    utmSource: string;
    utmCampaign: string;
    utmContent: string;
    utmTerm: string;
    revenue: number;
    profile?: IServiceProfile;
    hasReplay?: boolean;
    groups?: string[];
  }

  export interface IProfileMetrics {
    lastSeen: Date | null;
    firstSeen: Date | null;
    screenViews: number;
    sessions: number;
    durationAvg: number;
    durationP90: number;
    totalEvents: number;
    uniqueDaysActive: number;
    bounceRate: number;
    avgEventsPerSession: number;
    conversionEvents: number;
    avgTimeBetweenSessions: number;
    revenue: number;
  }

  export interface IServiceClient {
    id: string;
    name: string;
    projectId: string;
    secret: string;
    createdAt: Date | string;
    updatedAt: Date | string;
  }

  export interface IServiceProject {
    id: string;
    name: string;
    organizationId: string;
    createdAt: Date | string;
    updatedAt: Date | string;
    settings?: Record<string, unknown>;
  }

  export interface IServiceProjectWithClients extends IServiceProject {
    clients: IServiceClient[];
  }

  export interface IServiceOrganization {
    id: string;
    name: string;
    plan?: string;
    stripeCustomerId?: string | null;
    stripeSubscriptionId?: string | null;
    subscriptionEndsAt?: Date | string | null;
    createdAt: Date | string;
    updatedAt: Date | string;
    projects?: IServiceProject[];
    members?: IServiceMember[];
  }

  export interface IServiceMember {
    id: string;
    role: string;
    userId: string;
    organizationId: string;
    createdAt: Date | string;
    user?: {
      id: string;
      email: string;
      name?: string | null;
      avatar?: string | null;
    };
  }

  export interface IServiceDashboard {
    id: string;
    name: string;
    isDefault: boolean;
    projectId: string;
    organizationId: string;
    reports?: IServiceReport[];
    createdAt: Date | string;
    updatedAt: Date | string;
  }

  export type IServiceDashboards = IServiceDashboard[];

  export interface IServiceReport {
    id: string;
    name: string;
    chartType: string;
    lineType?: string;
    interval?: string;
    range?: string;
    series: any[];
    events: any[];
    breakdowns?: any[];
    layout?: {
      x: number;
      y: number;
      w: number;
      h: number;
      minW?: number;
      minH?: number;
    };
    [key: string]: any;
  }

  export interface IServiceGroup {
    id: string;
    name: string;
    type: string;
    createdAt: Date | string;
    lastActiveAt?: Date | string;
    properties?: Record<string, unknown>;
  }

  export interface IServiceReference {
    id: string;
    name: string;
    description?: string;
    date: Date | string;
    projectId: string;
  }

  export interface IGetTopGenericInput {
    projectId: string;
    range: string;
    interval?: string;
    startDate?: string;
    endDate?: string;
  }

  export interface INotificationPayload {
    id: string;
    name: string;
    [key: string]: any;
  }

  export interface Notification {
    id: string;
    title: string;
    body: string;
    createdAt: Date | string;
    [key: string]: any;
  }

  export interface NotificationRule {
    id: string;
    name: string;
    [key: string]: any;
  }
}
