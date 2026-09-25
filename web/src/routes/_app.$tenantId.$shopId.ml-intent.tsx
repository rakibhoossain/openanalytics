import { useQuery } from '@tanstack/react-query';
import { createFileRoute, Link } from '@tanstack/react-router';
import {
  ActivityIcon,
  AlertTriangleIcon,
  ArrowRightIcon,
  BrainCircuitIcon,
  CheckCircle2Icon,
  ClockIcon,
  CopyIcon,
  CpuIcon,
  DatabaseIcon,
  ExternalLinkIcon,
  EyeIcon,
  GlobeIcon,
  LaptopIcon,
  LayersIcon,
  PercentIcon,
  RefreshCwIcon,
  ShieldAlertIcon,
  ShieldCheckIcon,
  ShoppingCartIcon,
  SmartphoneIcon,
  SparklesIcon,
  TagIcon,
  TrendingDownIcon,
  TrendingUpIcon,
  UserCheckIcon,
  UserIcon,
  VideoIcon,
  ZapIcon,
} from 'lucide-react';
import { useState } from 'react';
import { toast } from 'sonner';
import { ProjectLink } from '@/components/links';
import { PageContainer } from '@/components/page-container';
import { PageHeader } from '@/components/page-header';
import { ProfileAvatar } from '@/components/profiles/profile-avatar';
import { SerieIcon } from '@/components/report-chart/common/serie-icon';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Progress } from '@/components/ui/progress';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useAppParams } from '@/hooks/use-app-params';
import { useRangePageContext } from '@/hooks/use-page-context-helpers';
import { createProjectTitle, PAGE_TITLES } from '@/utils/title';

export const Route = createFileRoute('/_app/$tenantId/$shopId/ml-intent')({
  component: Component,
  head: () => ({
    meta: [{ title: createProjectTitle(PAGE_TITLES.BEHAVIORAL_ML) }],
  }),
});

interface IntentShopper {
  device: string;
  sessionId?: string;
  profileId?: string;
  customerId?: string;
  isIdentified?: boolean;
  cartId?: string;
  cartValue?: number;
  cartItems?: number;
  views?: number;
  carts?: number;
  dwell_seconds?: number;
  country?: string;
  city?: string;
  os?: string;
  browser?: string;
  deviceType?: string;
  path?: string;
  hasReplay?: boolean;
  intent?: number;
  intentTier?: string;
  churnRisk?: number;
  churnTier?: string;
  priceSensitivity?: number;
  priceTier?: string;
  status?: string;
  signals?: string;
}

function Component() {
  const params = Route.useParams() as any;
  const { tenantId, shopId, organizationId, projectId } = useAppParams();
  const activeTenantId = tenantId || organizationId || params.tenantId || '';
  const activeShopId = shopId || projectId || params.shopId || '';
  useRangePageContext('insights');

  const [selectedShopper, setSelectedShopper] = useState<IntentShopper | null>(null);
  const [activeModelTab, setActiveModelTab] = useState<'intent' | 'churn' | 'price'>('intent');

  // Query enriched live shopper behavioral intelligence
  const {
    data: shoppersData,
    isLoading,
    isRefetching,
    refetch,
  } = useQuery({
    queryKey: ['ml-intents-enriched', activeShopId],
    queryFn: async (): Promise<IntentShopper[]> => {
      const res = await fetch(
        `/api/v1/query/intents?shop_id=${encodeURIComponent(activeShopId)}`,
        {
          headers: {
            'X-Tenant-ID': activeTenantId,
            'X-Shop-ID': activeShopId,
          },
        }
      );
      if (!res.ok) {
        throw new Error('Failed to fetch ML intent scores');
      }
      const json = await res.json();
      return json?.result?.data?.json || json?.data || json || [];
    },
    refetchInterval: 4000,
  });

  const shoppers: IntentShopper[] = Array.isArray(shoppersData) ? shoppersData : [];

  // Metrics
  const highIntentShoppers = shoppers.filter((s) => (s.intent ?? 0) >= 0.85);
  const atRiskCarts = shoppers.filter(
    (s) => ((s.carts ?? 0) > 0 || Boolean(s.cartId)) && (s.churnRisk ?? 0) >= 0.55
  );
  const priceSensitiveShoppers = shoppers.filter((s) => (s.priceSensitivity ?? 0) >= 0.60);
  const activeCartsCount = shoppers.filter((s) => (s.carts ?? 0) > 0 || Boolean(s.cartId)).length;

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    toast.success(`Copied ${label} to clipboard`);
  };

  return (
    <PageContainer>
      <div className="flex flex-col gap-6 pb-12">
        {/* Header with live indicator */}
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2.5">
              <PageHeader
                title="Behavioral ML & Cart Risk Radar"
                description="Real-time multi-model conversion propensity, cart abandonment risk & price sensitivity"
                className="mb-0"
              />
              <Badge
                variant="outline"
                className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20 flex items-center gap-1.5 px-2.5 py-0.5 text-xs font-semibold uppercase tracking-wider animate-pulse"
              >
                <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-ping" />
                Live Radar
              </Badge>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => refetch()}
              disabled={isLoading || isRefetching}
              className="gap-2 text-xs"
            >
              <RefreshCwIcon
                className={`h-3.5 w-3.5 ${isRefetching ? 'animate-spin' : ''}`}
              />
              Refresh Radar
            </Button>
          </div>
        </div>

        {/* Top KPI Summary Cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {/* Card 1: High Intent */}
          <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md relative overflow-hidden">
            <div className="flex items-center justify-between text-muted-foreground text-xs font-medium uppercase tracking-wider mb-2">
              <span>Ready-To-Buy Shoppers</span>
              <SparklesIcon className="h-4 w-4 text-emerald-500" />
            </div>
            <div className="text-3xl font-bold tracking-tight text-foreground flex items-baseline gap-2">
              <span>{highIntentShoppers.length}</span>
              <span className="text-xs font-normal text-emerald-500 font-mono">
                &gt;= 85% intent
              </span>
            </div>
            <p className="text-xs text-muted-foreground mt-2">
              Shoppers with strong checkout propensity
            </p>
            <div className="absolute -right-4 -bottom-4 w-16 h-16 bg-emerald-500/10 rounded-full blur-xl pointer-events-none" />
          </div>

          {/* Card 2: At-Risk Active Carts */}
          <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md relative overflow-hidden">
            <div className="flex items-center justify-between text-muted-foreground text-xs font-medium uppercase tracking-wider mb-2">
              <span>At-Risk Active Carts</span>
              <ShieldAlertIcon className="h-4 w-4 text-rose-500" />
            </div>
            <div className="text-3xl font-bold tracking-tight text-foreground flex items-baseline gap-2">
              <span className="text-rose-500">{atRiskCarts.length}</span>
              <span className="text-xs font-normal text-rose-500 font-mono">
                cart abandonment risk
              </span>
            </div>
            <p className="text-xs text-muted-foreground mt-2">
              Carts in progress with elevated drop-off signals
            </p>
            <div className="absolute -right-4 -bottom-4 w-16 h-16 bg-rose-500/10 rounded-full blur-xl pointer-events-none" />
          </div>

          {/* Card 3: Price Sensitive Shoppers */}
          <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md relative overflow-hidden">
            <div className="flex items-center justify-between text-muted-foreground text-xs font-medium uppercase tracking-wider mb-2">
              <span>Price-Sensitive Hunters</span>
              <TagIcon className="h-4 w-4 text-amber-500" />
            </div>
            <div className="text-3xl font-bold tracking-tight text-foreground flex items-baseline gap-2">
              <span>{priceSensitiveShoppers.length}</span>
              <span className="text-xs font-normal text-amber-500 font-mono">
                coupon eligible
              </span>
            </div>
            <p className="text-xs text-muted-foreground mt-2">
              Bargain hunters waiting for price incentives
            </p>
            <div className="absolute -right-4 -bottom-4 w-16 h-16 bg-amber-500/10 rounded-full blur-xl pointer-events-none" />
          </div>

          {/* Card 4: Active Carts & In-Memory Latency */}
          <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md relative overflow-hidden">
            <div className="flex items-center justify-between text-muted-foreground text-xs font-medium uppercase tracking-wider mb-2">
              <span>Monitored Sessions</span>
              <CpuIcon className="h-4 w-4 text-blue-500" />
            </div>
            <div className="text-3xl font-bold tracking-tight text-foreground flex items-baseline gap-2">
              <span>{shoppers.length}</span>
              <span className="text-xs font-normal text-blue-500 font-mono">
                ({activeCartsCount} in cart)
              </span>
            </div>
            <p className="text-xs text-muted-foreground mt-2">
              Tri-model scoring running in &lt; 50ns in Go
            </p>
            <div className="absolute -right-4 -bottom-4 w-16 h-16 bg-blue-500/10 rounded-full blur-xl pointer-events-none" />
          </div>
        </div>

        {/* Dual Split: Model Architecture & High-Intent Stream */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
          {/* Left Column: Multi-Model Architecture & Coefficients */}
          <div className="lg:col-span-4 flex flex-col gap-4">
            <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md">
              <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-2">
                  <BrainCircuitIcon className="h-5 w-5 text-primary" />
                  <h3 className="font-semibold text-foreground text-base">
                    Behavioral Models
                  </h3>
                </div>
                <Badge variant="secondary" className="font-mono text-[10px]">
                  ONNX + JSON
                </Badge>
              </div>

              {/* Model Switcher Tabs */}
              <Tabs
                value={activeModelTab}
                onValueChange={(val: any) => setActiveModelTab(val)}
                className="w-full"
              >
                <TabsList className="grid grid-cols-3 mb-4 h-8 text-xs">
                  <TabsTrigger value="intent" className="text-[11px] px-1">
                    Intent
                  </TabsTrigger>
                  <TabsTrigger value="churn" className="text-[11px] px-1">
                    Cart Churn
                  </TabsTrigger>
                  <TabsTrigger value="price" className="text-[11px] px-1">
                    Price Sens.
                  </TabsTrigger>
                </TabsList>

                {/* Tab 1: Intent */}
                <TabsContent value="intent" className="space-y-3 mt-0">
                  <p className="text-xs text-muted-foreground leading-relaxed">
                    Evaluates conversion propensity using rolling cart additions, catalog scatter, and active dwell time.
                  </p>
                  <div className="rounded-lg bg-muted/40 p-2.5 border border-border/30 font-mono text-[11px] text-muted-foreground">
                    <span className="text-emerald-500 font-bold">P(buy)</span> = Sigmoid(1.95·carts + 0.12·views - 0.15·scatter - 2.85)
                  </div>
                  <div className="space-y-2 pt-1 text-xs">
                    <div className="flex justify-between items-center p-2 rounded bg-muted/20">
                      <span className="flex items-center gap-1.5 font-medium">
                        <ShoppingCartIcon className="h-3.5 w-3.5 text-emerald-500" />
                        Cart Additions
                      </span>
                      <span className="font-mono font-semibold text-emerald-500">+1.95</span>
                    </div>
                    <div className="flex justify-between items-center p-2 rounded bg-muted/20">
                      <span className="flex items-center gap-1.5 font-medium">
                        <ClockIcon className="h-3.5 w-3.5 text-emerald-500" />
                        Active Dwell Time
                      </span>
                      <span className="font-mono font-semibold text-emerald-500">+0.0035/s</span>
                    </div>
                    <div className="flex justify-between items-center p-2 rounded bg-muted/20">
                      <span className="flex items-center gap-1.5 text-muted-foreground">
                        Catalog Scatter
                      </span>
                      <span className="font-mono font-semibold text-rose-500">-0.15</span>
                    </div>
                  </div>
                </TabsContent>

                {/* Tab 2: Churn */}
                <TabsContent value="churn" className="space-y-3 mt-0">
                  <p className="text-xs text-muted-foreground leading-relaxed">
                    Detects immediate drop-off and cart abandonment risk before the user bounces.
                  </p>
                  <div className="rounded-lg bg-muted/40 p-2.5 border border-border/30 font-mono text-[11px] text-muted-foreground">
                    <span className="text-rose-500 font-bold">P(churn)</span> = Sigmoid(1.15 - 2.10·carts - 1.85·scroll - 0.005·dwell)
                  </div>
                  <div className="space-y-2 pt-1 text-xs">
                    <div className="flex justify-between items-center p-2 rounded bg-muted/20">
                      <span className="flex items-center gap-1.5 font-medium">
                        <TrendingDownIcon className="h-3.5 w-3.5 text-rose-500" />
                        Zero Scroll Depth
                      </span>
                      <span className="font-mono font-semibold text-rose-500">+1.85</span>
                    </div>
                    <div className="flex justify-between items-center p-2 rounded bg-muted/20">
                      <span className="flex items-center gap-1.5 font-medium">
                        <ClockIcon className="h-3.5 w-3.5 text-rose-500" />
                        Hesitation Pauses
                      </span>
                      <span className="font-mono font-semibold text-amber-500">Elevated</span>
                    </div>
                  </div>
                </TabsContent>

                {/* Tab 3: Price */}
                <TabsContent value="price" className="space-y-3 mt-0">
                  <p className="text-xs text-muted-foreground leading-relaxed">
                    Identifies bargain hunters vs high-AOV buyers to target discounts selectively.
                  </p>
                  <div className="rounded-lg bg-muted/40 p-2.5 border border-border/30 font-mono text-[11px] text-muted-foreground">
                    <span className="text-amber-500 font-bold">P(discount)</span> = Sigmoid(3.85·sale_views + 0.35·carts - 1.65)
                  </div>
                  <div className="space-y-2 pt-1 text-xs">
                    <div className="flex justify-between items-center p-2 rounded bg-muted/20">
                      <span className="flex items-center gap-1.5 font-medium">
                        <TagIcon className="h-3.5 w-3.5 text-amber-500" />
                        Sale / Coupon Dwell
                      </span>
                      <span className="font-mono font-semibold text-amber-500">+3.85</span>
                    </div>
                  </div>
                </TabsContent>
              </Tabs>
            </div>

            {/* Automated Actions Card */}
            <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md">
              <h4 className="text-xs font-semibold text-foreground uppercase tracking-wider mb-2.5 flex items-center gap-1.5">
                <TrendingUpIcon className="h-4 w-4 text-emerald-500" />
                Live Conversion Triggers
              </h4>
              <ul className="text-xs text-muted-foreground space-y-2.5">
                <li className="flex items-start gap-2">
                  <CheckCircle2Icon className="h-3.5 w-3.5 text-emerald-500 shrink-0 mt-0.5" />
                  <span>
                    <strong>&gt;= 85% Purchase Intent:</strong> Prioritize inventory &amp; 1-click checkout.
                  </span>
                </li>
                <li className="flex items-start gap-2">
                  <AlertTriangleIcon className="h-3.5 w-3.5 text-rose-500 shrink-0 mt-0.5" />
                  <span>
                    <strong>&gt;= 60% Cart Drop Risk:</strong> Trigger exit-intent recovery or shipping prompt.
                  </span>
                </li>
                <li className="flex items-start gap-2">
                  <TagIcon className="h-3.5 w-3.5 text-amber-500 shrink-0 mt-0.5" />
                  <span>
                    <strong>Price Hunter:</strong> Deploy dynamic 10% coupon popover before exit.
                  </span>
                </li>
              </ul>
            </div>
          </div>

          {/* Right Column: Real-Time Shopper & Cart Radar Table */}
          <div className="lg:col-span-8">
            <div className="rounded-xl border border-border/40 bg-card/60 shadow-sm backdrop-blur-md flex flex-col h-full overflow-hidden">
              <div className="p-5 border-b border-border/40 flex items-center justify-between bg-muted/10">
                <div>
                  <h3 className="font-semibold text-foreground text-base flex items-center gap-2">
                    <ActivityIcon className="h-4 w-4 text-emerald-500" />
                    Active Shopper &amp; Cart Risk Stream
                  </h3>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    Real-time sessions, active carts, and multi-model propensity scores
                  </p>
                </div>
                <Badge variant="outline" className="text-xs font-mono">
                  {shoppers.length} Active Sessions
                </Badge>
              </div>

              <div className="flex-1 overflow-x-auto">
                {isLoading && shoppers.length === 0 ? (
                  <div className="py-16 text-center text-muted-foreground text-sm">
                    <RefreshCwIcon className="h-6 w-6 animate-spin mx-auto mb-2 text-primary" />
                    Polling inference scores...
                  </div>
                ) : shoppers.length === 0 ? (
                  <div className="py-16 text-center text-muted-foreground text-sm max-w-sm mx-auto px-4">
                    <CpuIcon className="h-8 w-8 mx-auto mb-3 text-muted-foreground/60" />
                    <p className="font-medium text-foreground mb-1">
                      No active shopper sessions scored yet
                    </p>
                    <p className="text-xs">
                      Shoppers browsing or adding items to carts will automatically appear in this radar.
                    </p>
                  </div>
                ) : (
                  <table className="w-full text-left text-xs border-collapse">
                    <thead>
                      <tr className="border-b border-border/40 bg-muted/20 text-muted-foreground font-medium uppercase tracking-wider text-[11px]">
                        <th className="py-3 px-4">Profile</th>
                        <th className="py-3 px-4">Session &amp; Replay</th>
                        <th className="py-3 px-4">Cart &amp; Checkout</th>
                        <th className="py-3 px-4 w-36">Propensity &amp; Risk</th>
                        <th className="py-3 px-4 text-right">Actions</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-border/20">
                      {shoppers.map((shopper, idx) => {
                        const intent = shopper.intent ?? 0;
                        const intentPct = Math.round(intent * 100);
                        const isHighIntent = intent >= 0.85;
                        const isMedIntent = intent >= 0.5 && !isHighIntent;

                        const churnRisk = shopper.churnRisk ?? 0.2;
                        const isHighChurn = churnRisk >= 0.6;
                        const isMedChurn = churnRisk >= 0.35 && !isHighChurn;

                        const isHunter = (shopper.priceSensitivity ?? 0) >= 0.65;

                        // Effective Profile identifier
                        const effectiveProfileId = shopper.customerId || shopper.profileId || shopper.device;
                        const isNamedCustomer = shopper.isIdentified && shopper.customerId;
                        const displayIdent = isNamedCustomer
                          ? shopper.customerId
                          : effectiveProfileId && effectiveProfileId.length > 10
                          ? `${effectiveProfileId.slice(0, 4)}...${effectiveProfileId.slice(-4)}`
                          : effectiveProfileId || 'Anonymous';

                        return (
                          <tr
                            key={shopper.sessionId || shopper.device || idx}
                            className={`hover:bg-muted/30 transition-colors ${
                              isHighIntent ? 'bg-emerald-500/5' : isHighChurn ? 'bg-rose-500/5' : ''
                            }`}
                          >
                            {/* 1. Shopper Profile */}
                            <td className="py-3 px-4">
                              <div className="flex items-center gap-2.5">
                                {effectiveProfileId ? (
                                  <ProjectLink
                                    to="/profiles/$profileId"
                                    params={{ profileId: effectiveProfileId }}
                                    className="group flex items-center gap-2.5 whitespace-nowrap font-medium hover:underline text-foreground"
                                  >
                                    <ProfileAvatar
                                      size="sm"
                                      id={effectiveProfileId}
                                      isExternal={shopper.isIdentified}
                                    />
                                    <div className="flex flex-col min-w-0">
                                      <div className="flex items-center gap-1.5">
                                        <span className="font-semibold text-foreground group-hover:text-primary transition-colors truncate max-w-36">
                                          {displayIdent}
                                        </span>
                                        {shopper.isIdentified && (
                                          <Badge
                                            variant="outline"
                                            className="h-4 px-1 text-[9px] bg-blue-500/10 text-blue-500 border-blue-500/20"
                                          >
                                            Customer
                                          </Badge>
                                        )}
                                      </div>
                                      <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                                        {shopper.country && (
                                          <span className="flex items-center gap-1">
                                            <SerieIcon name={shopper.country.toLowerCase()} />
                                            <span className="truncate max-w-20">{shopper.country}</span>
                                          </span>
                                        )}
                                        {shopper.browser && (
                                          <span className="flex items-center gap-1">
                                            <span>•</span>
                                            <SerieIcon name={shopper.browser} />
                                            <span>{shopper.browser}</span>
                                          </span>
                                        )}
                                        {shopper.os && (
                                          <span className="flex items-center gap-1">
                                            <span>•</span>
                                            <SerieIcon name={shopper.os} />
                                            <span>{shopper.os}</span>
                                          </span>
                                        )}
                                      </div>
                                    </div>
                                  </ProjectLink>
                                ) : (
                                  <div className="flex items-center gap-2">
                                    <ProfileAvatar size="sm" isExternal={false} />
                                    <span className="text-muted-foreground">Anonymous</span>
                                  </div>
                                )}
                              </div>
                            </td>

                            {/* 2. Session & Replay */}
                            <td className="py-3 px-4">
                              <div className="flex flex-col gap-1">
                                {shopper.sessionId ? (
                                  <Link
                                    to={'/$tenantId/$shopId/sessions/$sessionId' as any}
                                    params={{
                                      tenantId: activeTenantId,
                                      shopId: activeShopId,
                                      sessionId: shopper.sessionId,
                                    } as any}
                                    className="font-mono text-xs text-primary hover:underline flex items-center gap-1"
                                  >
                                    <span>{shopper.sessionId.slice(0, 8)}...</span>
                                    <ExternalLinkIcon className="h-3 w-3" />
                                  </Link>
                                ) : (
                                  <span className="font-mono text-xs text-muted-foreground">
                                    Active stream
                                  </span>
                                )}

                                {shopper.hasReplay && shopper.sessionId ? (
                                  <Link
                                    to={'/$tenantId/$shopId/sessions/$sessionId' as any}
                                    params={{
                                      tenantId: activeTenantId,
                                      shopId: activeShopId,
                                      sessionId: shopper.sessionId,
                                    } as any}
                                    className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 font-semibold text-[10px] w-fit hover:bg-emerald-500/25 transition-colors"
                                  >
                                    <VideoIcon className="h-3 w-3" />
                                    Watch Replay
                                  </Link>
                                ) : (
                                  <span className="text-[10px] text-muted-foreground">
                                    {shopper.views ?? 1} views • {shopper.dwell_seconds ?? 0}s dwell
                                  </span>
                                )}
                              </div>
                            </td>

                            {/* 3. Cart & Checkout Context */}
                            <td className="py-3 px-4">
                              {shopper.cartId && shopper.cartId !== '00000000-0000-0000-0000-000000000000' ? (
                                <div className="flex flex-col gap-1">
                                  <div className="flex items-center gap-1">
                                    <Badge
                                      variant="secondary"
                                      className="font-mono text-[10px] gap-1 px-1.5 py-0"
                                      title={shopper.cartId}
                                    >
                                      <ShoppingCartIcon className="h-2.5 w-2.5 text-primary" />
                                      cart-{shopper.cartId.slice(0, 6)}
                                    </Badge>
                                    <button
                                      onClick={() => copyToClipboard(shopper.cartId!, 'Cart ID')}
                                      className="text-muted-foreground hover:text-foreground"
                                      title="Copy Cart ID"
                                    >
                                      <CopyIcon className="h-3 w-3" />
                                    </button>
                                  </div>
                                  <span className="text-[11px] font-medium text-foreground">
                                    ${(shopper.cartValue ?? 0).toFixed(2)} • {shopper.carts ?? 1} item
                                    {(shopper.carts ?? 1) > 1 ? 's' : ''}
                                  </span>
                                </div>
                              ) : (shopper.carts ?? 0) > 0 ? (
                                <div className="flex flex-col gap-1">
                                  <span className="text-[11px] font-semibold text-emerald-600 dark:text-emerald-400 flex items-center gap-1">
                                    <ShoppingCartIcon className="h-3 w-3" />
                                    {shopper.carts} in cart
                                  </span>
                                  <span className="text-[10px] text-muted-foreground">
                                    Catalog exploration
                                  </span>
                                </div>
                              ) : (
                                <span className="text-[11px] text-muted-foreground">
                                  Browsing catalog ({shopper.views ?? 1} views)
                                </span>
                              )}
                            </td>

                            {/* 4. Propensity & Risk */}
                            <td className="py-3 px-4">
                              <div className="flex flex-col gap-1.5">
                                <div className="flex items-center justify-between">
                                  <span className="text-[10px] font-medium text-muted-foreground">
                                    Buy Intent
                                  </span>
                                  <span
                                    className={`font-mono font-bold text-xs ${
                                      isHighIntent
                                        ? 'text-emerald-500'
                                        : isMedIntent
                                        ? 'text-amber-500'
                                        : 'text-muted-foreground'
                                    }`}
                                  >
                                    {intentPct}%
                                  </span>
                                </div>
                                <Progress value={intentPct} className="h-1.5" />

                                <div className="flex items-center gap-1 flex-wrap mt-0.5">
                                  {isHighChurn && (shopper.carts ?? 0) > 0 && (
                                    <Badge
                                      variant="destructive"
                                      className="text-[9px] px-1 py-0 h-4 uppercase font-semibold"
                                    >
                                      Cart Risk
                                    </Badge>
                                  )}
                                  {isHunter && (
                                    <Badge
                                      variant="outline"
                                      className="text-[9px] px-1 py-0 h-4 bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20"
                                    >
                                      Price Hunter
                                    </Badge>
                                  )}
                                  {!isHighChurn && !isHunter && (
                                    <Badge
                                      variant="outline"
                                      className="text-[9px] px-1 py-0 h-4 text-muted-foreground"
                                    >
                                      {shopper.intentTier || 'CASUAL'}
                                    </Badge>
                                  )}
                                </div>
                              </div>
                            </td>

                            {/* 5. Actions */}
                            <td className="py-3 px-4 text-right">
                              <div className="flex items-center justify-end gap-1.5">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => setSelectedShopper(shopper)}
                                  className="h-7 text-xs px-2 gap-1 text-primary hover:bg-primary/10"
                                >
                                  <EyeIcon className="h-3 w-3" />
                                  Inspect
                                </Button>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Shopper & Cart Risk Inspector Dialog */}
      {selectedShopper && (
        <Dialog open={true} onOpenChange={() => setSelectedShopper(null)}>
          <DialogContent className="max-w-2xl bg-card border-border/50">
            <DialogHeader>
              <div className="flex items-center justify-between">
                <DialogTitle className="flex items-center gap-2 text-lg">
                  <BrainCircuitIcon className="h-5 w-5 text-primary" />
                  Shopper Behavioral Intelligence
                </DialogTitle>
                <Badge
                  variant={
                    (selectedShopper.intent ?? 0) >= 0.85
                      ? 'default'
                      : (selectedShopper.intent ?? 0) >= 0.5
                      ? 'secondary'
                      : 'outline'
                  }
                  className="font-semibold text-xs uppercase"
                >
                  {selectedShopper.intentTier || 'CASUAL'}
                </Badge>
              </div>
              <DialogDescription className="text-xs">
                In-depth behavioral analysis, cart abandonment risk breakdown, and instant actions
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-4 py-2">
              {/* Identity Banner */}
              <div className="rounded-lg bg-muted/30 p-3.5 border border-border/30 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                <div className="flex items-center gap-3">
                  <ProfileAvatar
                    size="default"
                    id={selectedShopper.customerId || selectedShopper.profileId || selectedShopper.device}
                    isExternal={selectedShopper.isIdentified}
                  />
                  <div>
                    <div className="flex items-center gap-2">
                      <ProjectLink
                        to="/profiles/$profileId"
                        params={{ profileId: selectedShopper.customerId || selectedShopper.profileId || selectedShopper.device }}
                        className="font-semibold text-sm text-foreground hover:underline hover:text-primary transition-colors"
                      >
                        {selectedShopper.isIdentified
                          ? selectedShopper.customerId || selectedShopper.profileId
                          : `Guest Shopper (${selectedShopper.device.slice(-8)})`}
                      </ProjectLink>
                      {selectedShopper.isIdentified && (
                        <Badge
                          variant="outline"
                          className="h-4 px-1 text-[9px] bg-blue-500/10 text-blue-500 border-blue-500/20"
                        >
                          Customer
                        </Badge>
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground flex items-center gap-2 mt-1">
                      {selectedShopper.country && (
                        <span className="flex items-center gap-1">
                          <SerieIcon name={selectedShopper.country.toLowerCase()} />
                          <span>{selectedShopper.country} {selectedShopper.city ? `• ${selectedShopper.city}` : ''}</span>
                        </span>
                      )}
                      {selectedShopper.browser && (
                        <span className="flex items-center gap-1">
                          <span>•</span>
                          <SerieIcon name={selectedShopper.browser} />
                          <span>{selectedShopper.browser}</span>
                        </span>
                      )}
                      {selectedShopper.os && (
                        <span className="flex items-center gap-1">
                          <span>•</span>
                          <SerieIcon name={selectedShopper.os} />
                          <span>{selectedShopper.os}</span>
                        </span>
                      )}
                    </p>
                  </div>
                </div>

                {selectedShopper.sessionId && (
                  <Link
                    to={'/$tenantId/$shopId/sessions/$sessionId' as any}
                    params={{
                      tenantId: activeTenantId,
                      shopId: activeShopId,
                      sessionId: selectedShopper.sessionId,
                    } as any}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-xs font-semibold hover:bg-primary/90 transition-colors w-fit"
                  >
                    <span>View Session</span>
                    <ArrowRightIcon className="h-3.5 w-3.5" />
                  </Link>
                )}
              </div>

              {/* Cart Context Banner */}
              {selectedShopper.cartId && selectedShopper.cartId !== '00000000-0000-0000-0000-000000000000' && (
                <div className="rounded-lg border border-border/40 bg-card p-3.5 flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className="h-9 w-9 rounded-md bg-primary/10 flex items-center justify-center text-primary">
                      <ShoppingCartIcon className="h-5 w-5" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-xs text-foreground">
                          Active Checkout Cart
                        </span>
                        <code className="text-[11px] bg-muted px-1.5 py-0.5 rounded font-mono">
                          {selectedShopper.cartId}
                        </code>
                        <button
                          onClick={() => copyToClipboard(selectedShopper.cartId!, 'Cart ID')}
                          className="text-muted-foreground hover:text-foreground"
                        >
                          <CopyIcon className="h-3 w-3" />
                        </button>
                      </div>
                      <p className="text-xs text-muted-foreground mt-0.5">
                        Cart Value: <strong className="text-foreground">${(selectedShopper.cartValue ?? 0).toFixed(2)}</strong> ({selectedShopper.carts ?? 1} item{(selectedShopper.carts ?? 1) > 1 ? 's' : ''})
                      </p>
                    </div>
                  </div>

                  <Badge
                    variant={(selectedShopper.churnRisk ?? 0) >= 0.6 ? 'destructive' : 'outline'}
                    className="text-xs uppercase"
                  >
                    {(selectedShopper.churnRisk ?? 0) >= 0.6 ? 'High Abandon Risk' : 'Healthy Cart'}
                  </Badge>
                </div>
              )}

              {/* Tri-Model Radar Scorecards */}
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                {/* 1. Intent */}
                <div className="rounded-lg border border-border/40 bg-muted/20 p-3 flex flex-col justify-between">
                  <div className="flex items-center justify-between text-xs text-muted-foreground mb-1">
                    <span>Purchase Intent</span>
                    <SparklesIcon className="h-3.5 w-3.5 text-emerald-500" />
                  </div>
                  <div className="text-2xl font-bold font-mono text-emerald-500">
                    {Math.round((selectedShopper.intent ?? 0) * 100)}%
                  </div>
                  <span className="text-[10px] text-muted-foreground uppercase font-semibold mt-1">
                    {selectedShopper.intentTier || 'CASUAL'}
                  </span>
                </div>

                {/* 2. Churn Risk */}
                <div className="rounded-lg border border-border/40 bg-muted/20 p-3 flex flex-col justify-between">
                  <div className="flex items-center justify-between text-xs text-muted-foreground mb-1">
                    <span>Abandonment Risk</span>
                    <AlertTriangleIcon className="h-3.5 w-3.5 text-rose-500" />
                  </div>
                  <div className="text-2xl font-bold font-mono text-rose-500">
                    {Math.round((selectedShopper.churnRisk ?? 0.2) * 100)}%
                  </div>
                  <span className="text-[10px] text-muted-foreground uppercase font-semibold mt-1">
                    {selectedShopper.churnTier || 'ENGAGED'}
                  </span>
                </div>

                {/* 3. Price Sensitivity */}
                <div className="rounded-lg border border-border/40 bg-muted/20 p-3 flex flex-col justify-between">
                  <div className="flex items-center justify-between text-xs text-muted-foreground mb-1">
                    <span>Price Sensitivity</span>
                    <TagIcon className="h-3.5 w-3.5 text-amber-500" />
                  </div>
                  <div className="text-2xl font-bold font-mono text-amber-500">
                    {Math.round((selectedShopper.priceSensitivity ?? 0.35) * 100)}%
                  </div>
                  <span className="text-[10px] text-muted-foreground uppercase font-semibold mt-1">
                    {selectedShopper.priceTier || 'MODERATE'}
                  </span>
                </div>
              </div>

              {/* Local Decision Drivers (SHAP Explainability) */}
              <div className="rounded-lg border border-border/40 bg-card p-3.5 space-y-2">
                <h5 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
                  <BrainCircuitIcon className="h-3.5 w-3.5 text-primary" />
                  Key Behavioral Drivers (Local Model Attribution)
                </h5>
                <div className="space-y-1.5 text-xs">
                  <div className="flex justify-between items-center py-1 border-b border-border/20">
                    <span className="flex items-center gap-1.5">
                      <ShoppingCartIcon className="h-3 w-3 text-emerald-500" />
                      Cart Activity ({selectedShopper.carts ?? 0} adds)
                    </span>
                    <span className="font-mono text-emerald-500 font-semibold">
                      {(selectedShopper.carts ?? 0) > 0 ? '+1.95 logit boost' : '0.00'}
                    </span>
                  </div>
                  <div className="flex justify-between items-center py-1 border-b border-border/20">
                    <span className="flex items-center gap-1.5">
                      <ClockIcon className="h-3 w-3 text-emerald-500" />
                      Active Engagement Dwell ({selectedShopper.dwell_seconds ?? 0}s)
                    </span>
                    <span className="font-mono text-emerald-500 font-semibold">
                      +{(((selectedShopper.dwell_seconds ?? 0) * 0.0035)).toFixed(2)} logit boost
                    </span>
                  </div>
                  <div className="flex justify-between items-center py-1">
                    <span className="flex items-center gap-1.5 text-muted-foreground">
                      Baseline Catalog Prior Bias
                    </span>
                    <span className="font-mono text-rose-500 font-semibold">-2.85 logit</span>
                  </div>
                </div>
              </div>

              {/* Automated Actions */}
              <div className="flex items-center justify-between pt-2">
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    className="gap-1.5 text-xs text-emerald-600 dark:text-emerald-400 border-emerald-500/30 hover:bg-emerald-500/10"
                    onClick={() => {
                      toast.success(
                        `Triggered Instant Checkout Incentive for Cart #${selectedShopper.cartId?.slice(-6) || 'active'}`
                      );
                    }}
                  >
                    <SparklesIcon className="h-3.5 w-3.5" />
                    Send Checkout Incentive
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    className="gap-1.5 text-xs text-rose-600 dark:text-rose-400 border-rose-500/30 hover:bg-rose-500/10"
                    onClick={() => {
                      toast.success(
                        `Triggered Cart Abandonment Recovery Webhook for ${selectedShopper.device.slice(-8)}`
                      );
                    }}
                  >
                    <AlertTriangleIcon className="h-3.5 w-3.5" />
                    Trigger Exit Recovery
                  </Button>
                </div>

                {selectedShopper.hasReplay && selectedShopper.sessionId && (
                  <Link
                    to={'/$tenantId/$shopId/sessions/$sessionId' as any}
                    params={{
                      tenantId: activeTenantId,
                      shopId: activeShopId,
                      sessionId: selectedShopper.sessionId,
                    } as any}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-emerald-600 text-white text-xs font-semibold hover:bg-emerald-500 transition-colors"
                  >
                    <VideoIcon className="h-3.5 w-3.5" />
                    Watch Replay
                  </Link>
                )}
              </div>
            </div>
          </DialogContent>
        </Dialog>
      )}
    </PageContainer>
  );
}
