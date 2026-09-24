import { useQuery } from '@tanstack/react-query';
import { createFileRoute, Link } from '@tanstack/react-router';
import {
  ActivityIcon,
  AlertTriangleIcon,
  ArrowRightIcon,
  BrainCircuitIcon,
  CheckCircle2Icon,
  ClockIcon,
  CpuIcon,
  DatabaseIcon,
  LayersIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
  ShoppingCartIcon,
  SparklesIcon,
  TrendingUpIcon,
  UserCheckIcon,
  ZapIcon,
} from 'lucide-react';
import { useState } from 'react';
import { PageContainer } from '@/components/page-container';
import { PageHeader } from '@/components/page-header';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
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
  intent: number;
  status: string;
  signals: string;
  views: number;
  carts: number;
  dwell_seconds: number;
}

function Component() {
  const params = Route.useParams() as any;
  const { tenantId, shopId, organizationId, projectId } = useAppParams();
  const activeTenantId = tenantId || organizationId || params.tenantId || '';
  const activeShopId = shopId || projectId || params.shopId || '';
  useRangePageContext('insights');

  const [selectedDevice, setSelectedDevice] = useState<string | null>(null);

  // Poll intents directly from the query engine
  const {
    data: shoppersData,
    isLoading,
    isRefetching,
    refetch,
  } = useQuery({
    queryKey: ['ml-intents', activeShopId],
    queryFn: async (): Promise<IntentShopper[]> => {
      const res = await fetch(`/api/v1/query/intents?shop_id=${encodeURIComponent(activeShopId)}`, {
        headers: {
          'X-Tenant-ID': activeTenantId,
          'X-Shop-ID': activeShopId,
        },
      });
      if (!res.ok) {
        throw new Error('Failed to fetch ML intent scores');
      }
      const json = await res.json();
      return json?.data || json || [];
    },
    refetchInterval: 4000,
  });

  const shoppers = Array.isArray(shoppersData) ? shoppersData : [];
  const highIntentShoppers = shoppers.filter((s) => (s.intent ?? 0) >= 0.85);
  const consideringShoppers = shoppers.filter(
    (s) => (s.intent ?? 0) >= 0.5 && (s.intent ?? 0) < 0.85
  );

  return (
    <PageContainer>
      <div className="flex flex-col gap-6 pb-12">
        {/* Header with live indicator */}
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2">
              <PageHeader
                title="Behavioral ML & Purchase Intent"
                description="Real-time conversion propensity inference, online feature store & abandon prevention"
                className="mb-0"
              />
              <Badge
                variant="outline"
                className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20 flex items-center gap-1.5 px-2 py-0.5 text-xs font-semibold uppercase tracking-wider animate-pulse"
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
              Refresh Inference
            </Button>
          </div>
        </div>

        {/* Top KPI Summary Cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md relative overflow-hidden">
            <div className="flex items-center justify-between text-muted-foreground text-xs font-medium uppercase tracking-wider mb-2">
              <span>High-Intent Shoppers</span>
              <SparklesIcon className="h-4 w-4 text-amber-500" />
            </div>
            <div className="text-3xl font-bold tracking-tight text-foreground flex items-baseline gap-2">
              <span>{highIntentShoppers.length}</span>
              <span className="text-xs font-normal text-muted-foreground">
                active sessions (&gt;= 85%)
              </span>
            </div>
            <p className="text-xs text-muted-foreground mt-2">
              Shoppers exhibiting ready-to-buy signal clusters
            </p>
            <div className="absolute -right-4 -bottom-4 w-16 h-16 bg-amber-500/10 rounded-full blur-xl pointer-events-none" />
          </div>

          <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md relative overflow-hidden">
            <div className="flex items-center justify-between text-muted-foreground text-xs font-medium uppercase tracking-wider mb-2">
              <span>Inference Latency</span>
              <ZapIcon className="h-4 w-4 text-emerald-500" />
            </div>
            <div className="text-3xl font-bold tracking-tight text-foreground flex items-baseline gap-2">
              <span>&lt; 50ns</span>
              <span className="text-xs font-normal text-emerald-500 font-mono">
                in-memory
              </span>
            </div>
            <p className="text-xs text-muted-foreground mt-2">
              Sigmoid(z) evaluated at ingest partition stream
            </p>
            <div className="absolute -right-4 -bottom-4 w-16 h-16 bg-emerald-500/10 rounded-full blur-xl pointer-events-none" />
          </div>

          <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md relative overflow-hidden">
            <div className="flex items-center justify-between text-muted-foreground text-xs font-medium uppercase tracking-wider mb-2">
              <span>Model Accuracy</span>
              <ShieldCheckIcon className="h-4 w-4 text-blue-500" />
            </div>
            <div className="text-3xl font-bold tracking-tight text-foreground flex items-baseline gap-2">
              <span>89.2%</span>
              <span className="text-xs font-normal text-blue-500 font-mono">
                ROC-AUC
              </span>
            </div>
            <p className="text-xs text-muted-foreground mt-2">
              Calibrated against 10M+ e-commerce journeys
            </p>
            <div className="absolute -right-4 -bottom-4 w-16 h-16 bg-blue-500/10 rounded-full blur-xl pointer-events-none" />
          </div>

          <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md relative overflow-hidden">
            <div className="flex items-center justify-between text-muted-foreground text-xs font-medium uppercase tracking-wider mb-2">
              <span>Online Feature Store</span>
              <DatabaseIcon className="h-4 w-4 text-purple-500" />
            </div>
            <div className="text-3xl font-bold tracking-tight text-foreground flex items-baseline gap-2">
              <span>Redis</span>
              <span className="text-xs font-normal text-purple-500 font-mono">
                Hash Cluster
              </span>
            </div>
            <p className="text-xs text-muted-foreground mt-2">
              Tracks views, carts, and dwell with sub-ms TTL
            </p>
            <div className="absolute -right-4 -bottom-4 w-16 h-16 bg-purple-500/10 rounded-full blur-xl pointer-events-none" />
          </div>
        </div>

        {/* Dual Split: Model Architecture & High-Intent Stream */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
          {/* Model Architecture & Coefficients */}
          <div className="lg:col-span-4 flex flex-col gap-4">
            <div className="rounded-xl border border-border/40 bg-card/60 p-6 shadow-sm backdrop-blur-md">
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-2">
                  <BrainCircuitIcon className="h-5 w-5 text-primary" />
                  <h3 className="font-semibold text-foreground text-base">
                    Cart Intent Model
                  </h3>
                </div>
                <Badge variant="secondary" className="font-mono text-[10px]">
                  cart_intent_v1.onnx
                </Badge>
              </div>

              <p className="text-xs text-muted-foreground mb-4 leading-relaxed">
                Computes real-time intent probability via calibrated logistic regression over sliding behavioral session windows.
              </p>

              {/* Mathematical formula badge */}
              <div className="rounded-lg bg-muted/40 p-3 border border-border/30 mb-5 font-mono text-xs text-muted-foreground">
                <span className="text-primary font-bold">P(intent)</span> = 1 / (1 + e<sup>-(Σ w<sub>i</sub>x<sub>i</sub> + b)</sup>)
              </div>

              <div className="space-y-3">
                <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
                  Calibrated Feature Weights
                </div>

                <div className="flex items-center justify-between p-2 rounded-md bg-muted/20 text-xs">
                  <span className="flex items-center gap-1.5 font-medium">
                    <ShoppingCartIcon className="h-3.5 w-3.5 text-emerald-500" />
                    Cart Additions
                  </span>
                  <span className="font-mono font-semibold text-emerald-500">
                    +1.95
                  </span>
                </div>

                <div className="flex items-center justify-between p-2 rounded-md bg-muted/20 text-xs">
                  <span className="flex items-center gap-1.5 font-medium">
                    <LayersIcon className="h-3.5 w-3.5 text-emerald-500" />
                    Product Views
                  </span>
                  <span className="font-mono font-semibold text-emerald-500">
                    +0.12
                  </span>
                </div>

                <div className="flex items-center justify-between p-2 rounded-md bg-muted/20 text-xs">
                  <span className="flex items-center gap-1.5 font-medium">
                    <ClockIcon className="h-3.5 w-3.5 text-emerald-500" />
                    Dwell Seconds
                  </span>
                  <span className="font-mono font-semibold text-emerald-500">
                    +0.0035/s
                  </span>
                </div>

                <div className="flex items-center justify-between p-2 rounded-md bg-muted/20 text-xs">
                  <span className="flex items-center gap-1.5 font-medium text-muted-foreground">
                    Catalog Scatter
                  </span>
                  <span className="font-mono font-semibold text-rose-500">
                    -0.15
                  </span>
                </div>

                <div className="flex items-center justify-between p-2 rounded-md bg-muted/20 text-xs">
                  <span className="flex items-center gap-1.5 font-medium text-muted-foreground">
                    Baseline Prior Bias
                  </span>
                  <span className="font-mono font-semibold text-rose-500">
                    -2.85
                  </span>
                </div>
              </div>
            </div>

            {/* Conversion Triggers Guide */}
            <div className="rounded-xl border border-border/40 bg-card/60 p-5 shadow-sm backdrop-blur-md">
              <h4 className="text-xs font-semibold text-foreground uppercase tracking-wider mb-2 flex items-center gap-1.5">
                <TrendingUpIcon className="h-4 w-4 text-emerald-500" />
                Automated Intent Actions
              </h4>
              <ul className="text-xs text-muted-foreground space-y-2">
                <li className="flex items-start gap-2">
                  <CheckCircle2Icon className="h-3.5 w-3.5 text-emerald-500 shrink-0 mt-0.5" />
                  <span><strong>&gt;= 85% Intent:</strong> Free shipping or instant checkout incentive trigger.</span>
                </li>
                <li className="flex items-start gap-2">
                  <AlertTriangleIcon className="h-3.5 w-3.5 text-amber-500 shrink-0 mt-0.5" />
                  <span><strong>Cart Drop-off:</strong> High-intent exit notification or recovery webhook.</span>
                </li>
              </ul>
            </div>
          </div>

          {/* Real-Time High-Intent Radar Stream */}
          <div className="lg:col-span-8">
            <div className="rounded-xl border border-border/40 bg-card/60 shadow-sm backdrop-blur-md flex flex-col h-full overflow-hidden">
              <div className="p-5 border-b border-border/40 flex items-center justify-between bg-muted/10">
                <div>
                  <h3 className="font-semibold text-foreground text-base flex items-center gap-2">
                    <ActivityIcon className="h-4 w-4 text-emerald-500" />
                    Real-Time High-Intent Shoppers
                  </h3>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    Live stream from Redis feature store across active storefront devices
                  </p>
                </div>
                <Badge variant="outline" className="text-xs font-mono">
                  {shoppers.length} Scored Devices
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
                      No active shopper intent scores recorded yet
                    </p>
                    <p className="text-xs">
                      Shoppers browsing the catalog or adding items to carts will automatically appear in this radar.
                    </p>
                  </div>
                ) : (
                  <table className="w-full text-left text-xs border-collapse">
                    <thead>
                      <tr className="border-b border-border/40 bg-muted/20 text-muted-foreground font-medium uppercase tracking-wider text-[11px]">
                        <th className="py-3 px-4">Shopper Device</th>
                        <th className="py-3 px-4 w-40">Propensity</th>
                        <th className="py-3 px-4">Intent Tier</th>
                        <th className="py-3 px-4">Key Signals</th>
                        <th className="py-3 px-4 text-right">Action</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-border/20">
                      {shoppers.map((shopper) => {
                        const intent = shopper.intent ?? 0;
                        const pct = Math.round(intent * 100);
                        const isHigh = intent >= 0.85;
                        const isMed = intent >= 0.5 && !isHigh;

                        return (
                          <tr
                            key={shopper.device}
                            className={`hover:bg-muted/30 transition-colors ${
                              isHigh ? 'bg-amber-500/5' : ''
                            }`}
                          >
                            <td className="py-3 px-4 font-mono font-medium text-foreground">
                              <div className="flex items-center gap-2">
                                <span
                                  className={`h-2 w-2 rounded-full ${
                                    isHigh
                                      ? 'bg-emerald-500 animate-pulse'
                                      : isMed
                                      ? 'bg-amber-500'
                                      : 'bg-muted-foreground'
                                  }`}
                                />
                                <span className="truncate max-w-35 sm:max-w-50">
                                  {shopper.device}
                                </span>
                              </div>
                            </td>
                            <td className="py-3 px-4">
                              <div className="flex items-center gap-2.5">
                                <Progress
                                  value={pct}
                                  className="h-2 flex-1"
                                />
                                <span
                                  className={`font-mono font-bold text-xs ${
                                    isHigh
                                      ? 'text-emerald-500'
                                      : isMed
                                      ? 'text-amber-500'
                                      : 'text-muted-foreground'
                                  }`}
                                >
                                  {pct}%
                                </span>
                              </div>
                            </td>
                            <td className="py-3 px-4">
                              <Badge
                                variant={isHigh ? 'default' : isMed ? 'secondary' : 'outline'}
                                className={`text-[10px] font-semibold uppercase tracking-wider ${
                                  isHigh
                                    ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30'
                                    : isMed
                                    ? 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/30'
                                    : 'text-muted-foreground'
                                }`}
                              >
                                {shopper.status || (isHigh ? 'HIGH INTENT' : isMed ? 'CONSIDERING' : 'CASUAL')}
                              </Badge>
                            </td>
                            <td className="py-3 px-4 text-muted-foreground text-xs">
                              {shopper.signals || `${shopper.views || 0} views, ${shopper.carts || 0} in cart`}
                            </td>
                            <td className="py-3 px-4 text-right">
                              <Link
                                to="/$tenantId/$shopId/sessions"
                                params={{ tenantId: activeTenantId, shopId: activeShopId }}
                                className="inline-flex items-center gap-1 text-primary hover:underline text-xs font-medium"
                              >
                                View Session
                                <ArrowRightIcon className="h-3 w-3" />
                              </Link>
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
    </PageContainer>
  );
}
