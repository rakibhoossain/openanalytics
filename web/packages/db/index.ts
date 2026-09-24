import { z } from 'zod';
import { zTimeInterval } from '@openpanel/validation';

export enum ProjectType {
  website = 'website',
  app = 'app',
  backend = 'backend',
}

export interface EventMeta {
  id: string;
  name: string;
  conversion?: boolean | null;
  color?: string | null;
  icon?: string | null;
  projectId: string;
  createdAt: Date;
  updatedAt: Date;
}

export interface IClickhouseEvent {
  id: string;
  name: string;
  device_id: string;
  profile_id: string;
  project_id: string;
  session_id: string;
  properties: Record<string, string>;
  created_at: string;
  country: string;
  city: string;
  region: string;
  longitude: number;
  latitude: number;
  os: string;
  os_version: string;
  browser: string;
  browser_version: string;
  device: string;
  brand: string;
  model: string;
  duration: number;
  path: string;
  origin: string;
  referrer: string;
  referrer_name: string;
  referrer_type: string;
  imported_at: string;
  sdk_name: string;
  sdk_version: string;
  revenue: number;
  groups: string[];
}

export interface IClickhouseProfile {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
  avatar: string;
  properties: Record<string, string | undefined>;
  project_id: string;
  is_external: boolean;
  created_at: string;
  last_seen_at: string;
  groups: string[];
}

export interface IServiceProfile {
  id: string;
  email: string;
  avatar: string;
  firstName: string;
  lastName: string;
  createdAt: Date;
  lastSeenAt: Date;
  isExternal: boolean;
  projectId: string;
  groups: string[];
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
  createdAt: Date;
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
  referrer: string | undefined;
  referrerName: string | undefined;
  referrerType: string | undefined;
  importedAt: Date | undefined;
  profile: IServiceProfile | undefined;
  meta: EventMeta | undefined;
  sdkName: string | undefined;
  sdkVersion: string | undefined;
  revenue?: number;
  groups: string[];
}

export interface IServiceEventMinimal {
  id: string;
  name: string;
  projectId: string;
  sessionId: string;
  createdAt: Date;
  country?: string | undefined;
  longitude?: number | undefined | null;
  latitude?: number | undefined | null;
  os?: string | undefined;
  browser?: string | undefined;
  device?: string | undefined;
  brand?: string | undefined;
  duration?: number;
  path: string;
  origin: string;
  referrer: string | undefined;
  meta: EventMeta | undefined;
  minimal: boolean;
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
  createdAt: Date;
  endedAt: Date;
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
  groups: string[];
}

export interface IServiceGroup {
  id: string;
  projectId: string;
  type: string;
  name: string;
  properties: Record<string, unknown>;
  createdAt: Date;
  updatedAt: Date;
}

export interface IServiceOrganization {
  id: string;
  name: string;
  createdByUserId?: string | null;
  timezone?: string | null;
  onboarding?: string | null;
  subscriptionId?: string | null;
  subscriptionCustomerId?: string | null;
  subscriptionPriceId?: string | null;
  subscriptionProductId?: string | null;
  subscriptionStatus?: string | null;
  subscriptionStartsAt?: Date | null;
  subscriptionEndsAt?: Date | null;
  subscriptionCanceledAt?: Date | null;
  subscriptionCreatedByUserId?: string | null;
  subscriptionPeriodEventsCount: number;
  subscriptionPeriodEventsCountExceededAt?: Date | null;
  subscriptionPeriodEventsLimit: number;
  subscriptionInterval?: string | null;
  deleteAt?: Date | null;
  createdAt: Date;
  updatedAt: Date;
  projects?: IServiceProject[];
}

export interface IServiceClient {
  id: string;
  name: string;
  secret?: string | null;
  type: string;
  projectId?: string | null;
  organizationId: string;
  ignoreCorsAndSecret: boolean;
  createdAt: Date;
  updatedAt: Date;
}

export interface IServiceProject {
  id: string;
  name: string;
  organizationId: string;
  eventsCount: number;
  types: ProjectType[];
  domain?: string | null;
  cors: string[];
  crossDomain: boolean;
  allowUnsafeRevenueTracking: boolean;
  filters: any;
  deleteAt?: Date | null;
  createdAt: Date;
  updatedAt: Date;
}

export interface IServiceProjectWithClients extends IServiceProject {
  clients: IServiceClient[];
}

export interface IServiceMember {
  id: string;
  role: string;
  email: string;
  userId?: string | null;
  invitedById?: string | null;
  organizationId: string;
  meta?: any;
  createdAt: Date;
  updatedAt: Date;
  user?: IServiceUser | null;
}

export interface IServiceUser {
  id: string;
  email: string;
  firstName?: string | null;
  lastName?: string | null;
  createdAt: Date;
  updatedAt: Date;
}

export interface IServiceReport {
  id: string;
  name: string;
  interval: string;
  range: string;
  chartType: string;
  lineType: string;
  breakdowns: any;
  events: any;
  globalFilters: any;
  formula?: string | null;
  unit?: string | null;
  metric: string;
  projectId: string;
  previous: boolean;
  criteria?: string | null;
  funnelGroup?: string | null;
  funnelWindow?: number | null;
  options?: any;
  visibleSeries: string[];
  startDate?: string | null;
  endDate?: string | null;
  dashboardId: string;
  layout?: any;
  createdAt: Date;
  updatedAt: Date;
}

export interface IServiceDashboard {
  id: string;
  name: string;
  organizationId: string;
  projectId: string;
  createdAt: Date;
  updatedAt: Date;
  reports?: IServiceReport[];
}

export type IServiceDashboards = IServiceDashboard[];

export interface IServiceReference {
  id: string;
  title: string;
  description?: string | null;
  date: Date;
  projectId: string;
  createdAt: Date;
  updatedAt: Date;
}

export interface Notification {
  id: string;
  projectId: string;
  title: string;
  message: string;
  isReadAt?: Date | null;
  createdAt: Date;
  updatedAt: Date;
  sendToApp: boolean;
  sendToEmail: boolean;
  integrationId?: string | null;
  notificationRuleId?: string | null;
  payload?: any;
}

export interface NotificationRule {
  id: string;
  name: string;
  projectId: string;
  sendToApp: boolean;
  sendToEmail: boolean;
  config: any;
  template?: string | null;
  createdAt: Date;
  updatedAt: Date;
  integrations?: any[];
}

export type INotificationPayload =
  | {
      type: 'event';
      event: any;
    }
  | {
      type: 'funnel';
      funnel: IServiceEvent[];
    };

export const APP_NOTIFICATION_INTEGRATION_ID = 'app';
export const EMAIL_NOTIFICATION_INTEGRATION_ID = 'email';
export const BASE_INTEGRATIONS: any[] = [];
export const isBaseIntegration = (id: string) => false;

export const TABLE_NAMES = {
  events: 'events',
  profiles: 'profiles',
  alias: 'profile_aliases',
  self_hosting: 'self_hosting',
  events_bots: 'events_bots',
  dau_mv: 'dau_mv',
  event_names_mv: 'distinct_event_names_mv',
  event_property_values_mv: 'event_property_values_mv',
  cohort_events_mv: 'cohort_events_mv',
  sessions: 'sessions',
  events_imports: 'events_imports',
  session_replay_chunks: 'session_replay_chunks',
  gsc_daily: 'gsc_daily',
  gsc_pages_daily: 'gsc_pages_daily',
  gsc_queries_daily: 'gsc_queries_daily',
  groups: 'groups',
  cohort_members: 'cohort_members',
  cohort_metadata: 'cohort_metadata',
  profile_event_summary_mv: 'profile_event_summary_mv',
  profile_event_property_summary_mv: 'profile_event_property_summary_mv',
};

export const SESSION_DISTINCT_FIELDS = [
  'referrer_name',
  'country',
  'os',
  'browser',
  'device',
] as const;

export type SessionDistinctField = (typeof SESSION_DISTINCT_FIELDS)[number];

export const zGetMetricsInput = z.object({
  projectId: z.string(),
  filters: z.array(z.any()),
  startDate: z.string(),
  endDate: z.string(),
  interval: zTimeInterval,
});
export type IGetMetricsInput = z.infer<typeof zGetMetricsInput> & { timezone: string };

export const zGetTopPagesInput = z.object({
  projectId: z.string(),
  filters: z.array(z.any()),
  startDate: z.string(),
  endDate: z.string(),
  limit: z.number().min(1).max(1000).optional(),
});
export type IGetTopPagesInput = z.infer<typeof zGetTopPagesInput> & { timezone: string };

export const zGetTopGenericInput = z.object({
  projectId: z.string(),
  filters: z.array(z.any()),
  startDate: z.string(),
  endDate: z.string(),
  column: z.enum([
    'referrer',
    'referrer_name',
    'referrer_type',
    'utm_source',
    'utm_medium',
    'utm_campaign',
    'utm_term',
    'utm_content',
    'region',
    'country',
    'city',
    'device',
    'brand',
    'model',
    'browser',
    'browser_version',
    'os',
    'os_version',
  ]),
});
export type IGetTopGenericInput = z.infer<typeof zGetTopGenericInput> & { timezone: string };

export const zGetTopGenericSeriesInput = zGetTopGenericInput.extend({
  interval: zTimeInterval,
});
export type IGetTopGenericSeriesInput = z.infer<typeof zGetTopGenericSeriesInput> & { timezone: string };

export const zGetUserJourneyInput = z.object({
  projectId: z.string(),
  filters: z.array(z.any()),
  startDate: z.string(),
  endDate: z.string(),
  steps: z.number().min(2).max(10).default(5),
});
export type IGetUserJourneyInput = z.infer<typeof zGetUserJourneyInput> & { timezone: string };

export const zGetTopEventsInput = z.object({
  projectId: z.string(),
  filters: z.array(z.any()),
  startDate: z.string(),
  endDate: z.string(),
  excludeEvents: z.array(z.string()).optional(),
});
export type IGetTopEventsInput = z.infer<typeof zGetTopEventsInput> & { timezone: string };

export const zGetTopLinkOutInput = z.object({
  projectId: z.string(),
  filters: z.array(z.any()),
  startDate: z.string(),
  endDate: z.string(),
});
export type IGetTopLinkOutInput = z.infer<typeof zGetTopLinkOutInput> & { timezone: string };

export const zGetMapDataInput = z.object({
  projectId: z.string(),
  filters: z.array(z.any()),
  startDate: z.string(),
  endDate: z.string(),
});
export type IGetMapDataInput = z.infer<typeof zGetMapDataInput> & { timezone: string };

// Namespace Prisma for Prisma types
export namespace Prisma {
  export type ProjectGetPayload<T> = any;
  export type NotificationUncheckedCreateInput = any;
  export type EventMetaSelect = any;
  export type JsonValue = any;
}

// Runtime proxy and stubs for @openpanel/trpc
const noop: any = (..._args: any[]) => ({} as any);
const asyncNoop: any = async (..._args: any[]) => ({} as any);

export const db: any = new Proxy(noop, {
  get: () => new Proxy(noop, {
    get: () => noop,
    apply: () => ({} as any),
  }),
  apply: () => ({} as any),
});
export const ch: any = db;
export const clix: any = db;
export const chQuery: any = asyncNoop;
export const eventBuffer: any = db;

export const getId = () => '';
export const runWithAlsSession = (_session: any, cb: any) => cb();
export const encrypt = (s: string) => s;
export const decrypt = (s: string) => s;
export const formatClickhouseDate = (d: Date) => d.toISOString();
export const convertClickhouseDateToJs = (d: string) => new Date(d);
export const toNullIfDefaultMinDate = (d: any) => d;
export const isKnownEventField = (_f: string) => false;
export const normalizeEventField = (f: string) => f;
export const getSelectPropertyKey = (k: string) => k;
export const onlyReportEvents = (e: any) => e;
export const mergeGlobalFilters = (..._args: any[]) => [];
export const createSqlBuilder = () => ({});

export const ChartEngine: any = class {};
export const AggregateChartEngine: any = class {};

export const overviewService: any = db;
export const sessionService: any = db;
export const eventService: any = db;
export const conversionService: any = db;
export const funnelService: any = db;
export const pagesService: any = db;
export const sankeyService: any = db;

export const getChartPrevStartEndDate: any = asyncNoop;
export const getChartStartEndDate: any = asyncNoop;
export const getConversionEventNames: any = asyncNoop;
export const getOrganizationSubscriptionChartEndDate: any = asyncNoop;
export const getReferrerSpikes: any = asyncNoop;
export const getSettingsForProject: any = asyncNoop;
export const validateOverviewShareAccess: any = asyncNoop;
export const validateShareAccess: any = asyncNoop;
export const getProjectAccess: any = asyncNoop;
export const getClientByIdCached: any = asyncNoop;
export const getCohortCount: any = asyncNoop;
export const getCohortEventsPerDay: any = asyncNoop;
export const getCohortMemberEvents: any = asyncNoop;
export const getCohortMemberRoutes: any = asyncNoop;
export const getCohortMembers: any = asyncNoop;
export const computeCohort: any = asyncNoop;
export const countCohort: any = asyncNoop;
export const deleteCohortMembership: any = asyncNoop;
export const enqueueCohortCompute: any = asyncNoop;
export const listCohortMemberProfiles: any = asyncNoop;
export const getConversationById: any = asyncNoop;
export const deleteConversation: any = asyncNoop;
export const listConversations: any = asyncNoop;
export const upsertConversationTitle: any = asyncNoop;
export const getDashboardById: any = asyncNoop;
export const getDashboardsByProjectId: any = asyncNoop;
export const getDatesFromRange: any = asyncNoop;
export const resolveDateRange: any = asyncNoop;
export const getEventFiltersWhereClause: any = asyncNoop;
export const getEventList: any = asyncNoop;
export const getEventMetasCached: any = asyncNoop;
export const listEventNamesCore: any = asyncNoop;
export const transformEvent: any = asyncNoop;
export const createGroup: any = asyncNoop;
export const deleteGroup: any = asyncNoop;
export const updateGroup: any = asyncNoop;
export const getGroupById: any = asyncNoop;
export const getGroupList: any = asyncNoop;
export const getGroupListCount: any = asyncNoop;
export const getGroupMemberProfiles: any = asyncNoop;
export const getGroupPropertyKeys: any = asyncNoop;
export const getGroupPropertySelect: any = asyncNoop;
export const getGroupStats: any = asyncNoop;
export const getGroupTypes: any = asyncNoop;
export const getGroupsByIds: any = asyncNoop;
export const getGscCannibalization: any = asyncNoop;
export const getGscOverview: any = asyncNoop;
export const getGscPageDetails: any = asyncNoop;
export const getGscPages: any = asyncNoop;
export const getGscQueries: any = asyncNoop;
export const getGscQueryDetails: any = asyncNoop;
export const listGscSites: any = asyncNoop;
export const getInviteById: any = asyncNoop;
export const getInvites: any = asyncNoop;
export const getIsRegistrationAllowed: any = asyncNoop;
export const getMembers: any = asyncNoop;
export const connectUserToOrganization: any = asyncNoop;
export const getNotificationRulesByProjectId: any = asyncNoop;
export const getOrganizationAccess: any = asyncNoop;
export const getOrganizationBillingEventsCountSerieCached: any = asyncNoop;
export const getOrganizationById: any = asyncNoop;
export const getOrganizationByProjectIdCached: any = asyncNoop;
export const getOrganizations: any = asyncNoop;
export const getProfileById: any = asyncNoop;
export const getProfileList: any = asyncNoop;
export const getProfileListCount: any = asyncNoop;
export const getProfileMetrics: any = asyncNoop;
export const getProfilePropertyKeysCached: any = asyncNoop;
export const getProfilePropertySelect: any = asyncNoop;
export const getProfiles: any = asyncNoop;
export const getProfilesCached: any = asyncNoop;
export const getProjectById: any = asyncNoop;
export const getProjectByIdCached: any = asyncNoop;
export const getProjectWithClients: any = asyncNoop;
export const getProjects: any = asyncNoop;
export const getReportById: any = asyncNoop;
export const getReportsByDashboardId: any = asyncNoop;
export const transformReport: any = asyncNoop;
export const getRetentionCohort: any = asyncNoop;
export const getSegmentDailySeriesCore: any = asyncNoop;
export const getSessionDistinctValues: any = asyncNoop;
export const getSessionList: any = asyncNoop;
export const getSessionReplayChunksFrom = async (
  sessionId: string,
  projectId: string,
  fromIndex = 0
) => {
  try {
    const apiUrl =
      process.env.API_URL_SSR || process.env.API_URL || 'http://localhost:8081';
    const res = await fetch(
      `${apiUrl}/trpc/session.replayChunksFrom?input=${encodeURIComponent(
        JSON.stringify({ sessionId, projectId, fromIndex })
      )}`
    );
    if (res.ok) {
      const json = await res.json();
      return json?.result?.data?.json ?? { data: [], hasMore: false };
    }
  } catch {}
  return { data: [], hasMore: false };
};
export const getShareDashboardById: any = asyncNoop;
export const getShareOverviewById: any = asyncNoop;
export const getShareReportById: any = asyncNoop;
export const getTopPagesCore: any = asyncNoop;
export const getTrafficBreakdownCore: any = asyncNoop;
export const getUserAccount: any = asyncNoop;
export const getUserById: any = asyncNoop;
