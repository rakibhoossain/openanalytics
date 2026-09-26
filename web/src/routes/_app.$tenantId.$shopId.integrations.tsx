import { createFileRoute } from '@tanstack/react-router';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, useEffect } from 'react';
import {
  CheckCircle2Icon,
  XCircleIcon,
  ShieldCheckIcon,
  KeyIcon,
  Share2Icon,
  ExternalLinkIcon,
  SendIcon,
  SaveIcon,
  SparklesIcon,
  EyeIcon,
  EyeOffIcon,
  LayersIcon,
  ArrowRightIcon,
  RadioIcon,
  ZapIcon,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { useAppParams } from '@/hooks/use-app-params';

export const Route = createFileRoute('/_app/$tenantId/$shopId/integrations')({
  component: IntegrationsPage,
  head: () => ({
    meta: [{ title: 'Integrations | OpenAnalytics' }],
  }),
});

interface MetaIntegrationData {
  shop_id?: string;
  tenant_id?: string;
  enabled: boolean;
  pixel_id: string;
  has_access_token: boolean;
  masked_token?: string;
  test_event_code?: string;
  events_whitelist: string[];
  updated_at?: string;
}

const AVAILABLE_EVENTS = [
  { id: 'PageView', label: 'PageView', desc: 'Site-wide visitor activity and page visits' },
  { id: 'ViewContent', label: 'ViewContent', desc: 'Product and catalog view details (PDP & Collection)' },
  { id: 'AddToCart', label: 'AddToCart', desc: 'Product added to shopping cart with price & quantity' },
  { id: 'InitiateCheckout', label: 'InitiateCheckout', desc: 'Shopper initiates checkout funnel' },
  { id: 'Purchase', label: 'Purchase', desc: 'Completed order with revenue, currency, and contents' },
];

export default function IntegrationsPage() {
  const { tenantId, shopId, organizationId, projectId } = useAppParams();
  const activeTenantId = tenantId || organizationId || '';
  const activeShopId = shopId || projectId || '';
  const queryClient = useQueryClient();

  const [enabled, setEnabled] = useState(false);
  const [pixelId, setPixelId] = useState('');
  const [accessToken, setAccessToken] = useState('');
  const [showToken, setShowToken] = useState(false);
  const [testEventCode, setTestEventCode] = useState('');
  const [whitelist, setWhitelist] = useState<string[]>([
    'PageView',
    'ViewContent',
    'AddToCart',
    'InitiateCheckout',
    'Purchase',
  ]);
  const [testResult, setTestResult] = useState<{
    success?: boolean;
    error?: string;
    eventsReceived?: number;
    fbtraceId?: string;
  } | null>(null);

  // 1. Fetch current Meta CAPI configuration
  const { data: configData, isLoading } = useQuery<{ success: boolean; data: MetaIntegrationData }>({
    queryKey: ['meta-capi-config', activeShopId],
    queryFn: async () => {
      const res = await fetch(`/api/v1/integrations/meta?shop_id=${activeShopId}`);
      if (!res.ok) throw new Error('Failed to load Meta configuration');
      return res.json();
    },
    enabled: Boolean(activeShopId),
  });

  useEffect(() => {
    if (configData?.data) {
      const d = configData.data;
      setEnabled(d.enabled);
      setPixelId(d.pixel_id || '');
      setAccessToken(d.has_access_token ? d.masked_token || '' : '');
      setTestEventCode(d.test_event_code || '');
      if (d.events_whitelist && d.events_whitelist.length > 0) {
        setWhitelist(d.events_whitelist);
      }
    }
  }, [configData]);

  // 2. Save Mutation
  const saveMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch('/api/v1/integrations/meta', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          shop_id: activeShopId,
          tenant_id: activeTenantId,
          enabled,
          pixel_id: pixelId.trim(),
          access_token: accessToken.trim(),
          test_event_code: testEventCode.trim(),
          events_whitelist: whitelist,
        }),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.message || 'Failed to save settings');
      }
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['meta-capi-config', activeShopId] });
      setTestResult(null);
    },
  });

  // 3. Test Connection Mutation
  const testMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch('/api/v1/integrations/meta/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          shop_id: activeShopId,
          pixel_id: pixelId.trim(),
          access_token: accessToken.trim(),
          test_event_code: testEventCode.trim(),
          event_name: 'PageView',
        }),
      });
      return res.json();
    },
    onSuccess: (data) => {
      if (data.success) {
        setTestResult({
          success: true,
          eventsReceived: data.events_received,
          fbtraceId: data.fbtrace_id,
        });
      } else {
        setTestResult({
          success: false,
          error: data.error || 'Meta API returned an error',
        });
      }
    },
    onError: (err: any) => {
      setTestResult({
        success: false,
        error: err.message || 'Failed to dispatch test event',
      });
    },
  });

  const toggleEvent = (eventId: string) => {
    setWhitelist((prev) =>
      prev.includes(eventId) ? prev.filter((id) => id !== eventId) : [...prev, eventId]
    );
  };

  const isConfigured = Boolean(configData?.data?.pixel_id && configData?.data?.has_access_token);

  return (
    <div className="flex-1 space-y-8 p-6 md:p-8 max-w-6xl mx-auto">
      {/* Top Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-border/40 pb-6">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl md:text-3xl font-bold tracking-tight">Integrations</h1>
            <Badge variant="outline" className="text-xs bg-primary/10 border-primary/20 text-primary">
              First-Party Engine
            </Badge>
          </div>
          <p className="text-muted-foreground text-sm mt-1">
            Stream server-side conversions directly to ad networks with zero 3rd-party proxies (Stape.io / SS-GTM).
          </p>
        </div>

        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => testMutation.mutate()}
            disabled={testMutation.isPending || (!pixelId && !isConfigured)}
            className="gap-2"
          >
            <SendIcon className="h-4 w-4" />
            {testMutation.isPending ? 'Sending...' : 'Send Test Event'}
          </Button>
          <Button
            size="sm"
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending || isLoading}
            className="gap-2"
          >
            <SaveIcon className="h-4 w-4" />
            {saveMutation.isPending ? 'Saving...' : 'Save Changes'}
          </Button>
        </div>
      </div>

      {/* Main Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
        {/* Left Column: Meta CAPI Settings */}
        <div className="lg:col-span-2 space-y-6">
          <div className="rounded-xl border border-border/60 bg-card p-6 shadow-sm relative overflow-hidden">
            {/* Top Card Bar */}
            <div className="flex items-center justify-between border-b border-border/40 pb-4 mb-6">
              <div className="flex items-center gap-3">
                <div className="h-10 w-10 rounded-lg bg-blue-600/10 flex items-center justify-center text-blue-500 font-bold border border-blue-500/20">
                  <Share2Icon className="h-5 w-5" />
                </div>
                <div>
                  <h2 className="font-semibold text-base flex items-center gap-2">
                    Meta Conversions API (CAPI)
                    {isConfigured && enabled && (
                      <span className="flex h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />
                    )}
                  </h2>
                  <p className="text-xs text-muted-foreground">
                    Direct server-side event transmission to Facebook & Instagram Ads
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-3">
                <span className="text-xs text-muted-foreground font-medium">
                  {enabled ? 'Enabled' : 'Disabled'}
                </span>
                <Switch checked={enabled} onCheckedChange={setEnabled} />
              </div>
            </div>

            {/* Form Fields */}
            <div className="space-y-5">
              {/* Pixel ID */}
              <div className="space-y-2">
                <label className="text-xs font-semibold text-foreground uppercase tracking-wider flex items-center justify-between">
                  <span>Dataset / Pixel ID</span>
                  <a
                    href="https://adsmanager.facebook.com/events_manager"
                    target="_blank"
                    rel="noreferrer"
                    className="text-xs font-normal text-primary hover:underline flex items-center gap-1"
                  >
                    Find in Events Manager <ExternalLinkIcon className="h-3 w-3" />
                  </a>
                </label>
                <Input
                  value={pixelId}
                  onChange={(e) => setPixelId(e.target.value)}
                  placeholder="e.g. 123456789012345"
                  className="font-mono text-sm"
                />
              </div>

              {/* Access Token */}
              <div className="space-y-2">
                <label className="text-xs font-semibold text-foreground uppercase tracking-wider flex items-center justify-between">
                  <span>Conversions API Access Token</span>
                  <span className="text-xs font-normal text-muted-foreground">
                    System User or App Token (SHA-256 protected)
                  </span>
                </label>
                <div className="relative">
                  <Input
                    type={showToken ? 'text' : 'password'}
                    value={accessToken}
                    onChange={(e) => setAccessToken(e.target.value)}
                    placeholder={configData?.data?.has_access_token ? '••••••••••••••••••••••••••••••••' : 'EAAB...'}
                    className="font-mono text-sm pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowToken(!showToken)}
                    className="absolute right-3 top-2.5 text-muted-foreground hover:text-foreground"
                  >
                    {showToken ? <EyeOffIcon className="h-4 w-4" /> : <EyeIcon className="h-4 w-4" />}
                  </button>
                </div>
                {configData?.data?.has_access_token && (
                  <p className="text-xs text-emerald-500/90 flex items-center gap-1 mt-1">
                    <ShieldCheckIcon className="h-3.5 w-3.5" /> Token securely configured. Leave unchanged to keep current token.
                  </p>
                )}
              </div>

              {/* Test Event Code */}
              <div className="space-y-2">
                <label className="text-xs font-semibold text-foreground uppercase tracking-wider flex items-center justify-between">
                  <span>Test Event Code (Optional)</span>
                  <span className="text-xs font-normal text-muted-foreground">
                    For debugging in Meta Events Manager &gt; Test Events
                  </span>
                </label>
                <Input
                  value={testEventCode}
                  onChange={(e) => setTestEventCode(e.target.value)}
                  placeholder="e.g. TEST12345"
                  className="font-mono text-sm uppercase"
                />
              </div>

              {/* Event Whitelist */}
              <div className="space-y-3 pt-2">
                <label className="text-xs font-semibold text-foreground uppercase tracking-wider block">
                  Events to Forward to Meta CAPI
                </label>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  {AVAILABLE_EVENTS.map((ev) => {
                    const isChecked = whitelist.includes(ev.id);
                    return (
                      <div
                        key={ev.id}
                        onClick={() => toggleEvent(ev.id)}
                        className={`flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-all ${
                          isChecked
                            ? 'border-primary/40 bg-primary/5 text-foreground'
                            : 'border-border/40 hover:border-border text-muted-foreground'
                        }`}
                      >
                        <Checkbox checked={isChecked} onCheckedChange={() => toggleEvent(ev.id)} />
                        <div>
                          <div className="font-semibold text-sm leading-none flex items-center gap-1.5">
                            {ev.label}
                            {ev.id === 'Purchase' && (
                              <Badge variant="secondary" className="text-[10px] py-0 px-1 font-mono">
                                ROAS
                              </Badge>
                            )}
                          </div>
                          <p className="text-xs text-muted-foreground mt-1">{ev.desc}</p>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>

            {/* Live Test Feedback Banner */}
            {testResult && (
              <div
                className={`mt-6 p-4 rounded-lg border text-sm flex items-start gap-3 ${
                  testResult.success
                    ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200'
                    : 'border-rose-500/30 bg-rose-500/10 text-rose-200'
                }`}
              >
                {testResult.success ? (
                  <CheckCircle2Icon className="h-5 w-5 text-emerald-400 shrink-0 mt-0.5" />
                ) : (
                  <XCircleIcon className="h-5 w-5 text-rose-400 shrink-0 mt-0.5" />
                )}
                <div className="space-y-1">
                  <div className="font-semibold">
                    {testResult.success
                      ? 'Meta Conversions API Verified Successfully!'
                      : 'Meta CAPI Verification Failed'}
                  </div>
                  {testResult.success ? (
                    <p className="text-xs text-emerald-300/80">
                      Delivered live test event to Meta Graph API. Events received:{' '}
                      <span className="font-mono">{testResult.eventsReceived}</span> | Trace ID:{' '}
                      <span className="font-mono">{testResult.fbtraceId}</span>. Check Meta Events Manager &gt; Test Events to view it live!
                    </p>
                  ) : (
                    <p className="text-xs text-rose-300/80 font-mono break-all">{testResult.error}</p>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Right Column: Architecture & First-Party Advantages */}
        <div className="space-y-6">
          {/* Status Overview Card */}
          <div className="rounded-xl border border-border/60 bg-card p-6 shadow-sm space-y-4">
            <h3 className="font-semibold text-sm flex items-center gap-2">
              <ZapIcon className="h-4 w-4 text-amber-400" /> Integration Status
            </h3>
            <div className="space-y-2 text-xs">
              <div className="flex items-center justify-between py-1 border-b border-border/40">
                <span className="text-muted-foreground">Status</span>
                {enabled && isConfigured ? (
                  <Badge variant="default" className="bg-emerald-500/20 text-emerald-400 border-emerald-500/30">
                    Active
                  </Badge>
                ) : isConfigured ? (
                  <Badge variant="outline" className="text-muted-foreground">
                    Disabled
                  </Badge>
                ) : (
                  <Badge variant="outline" className="text-amber-400 border-amber-400/30">
                    Not Configured
                  </Badge>
                )}
              </div>
              <div className="flex items-center justify-between py-1 border-b border-border/40">
                <span className="text-muted-foreground">Match Quality (EMQ)</span>
                <span className="font-medium text-emerald-400">High (SHA-256 Hashed)</span>
              </div>
              <div className="flex items-center justify-between py-1 border-b border-border/40">
                <span className="text-muted-foreground">Deduplication</span>
                <span className="font-mono text-primary">event_id + event_name</span>
              </div>
              <div className="flex items-center justify-between py-1 border-b border-border/40">
                <span className="text-muted-foreground">Relay Method</span>
                <span className="font-medium">Async Background Dispatch</span>
              </div>
              <div className="flex items-center justify-between py-1">
                <span className="text-muted-foreground">Graph API Version</span>
                <span className="font-mono">v19.0</span>
              </div>
            </div>
          </div>

          {/* Architecture Benefits */}
          <div className="rounded-xl border border-border/60 bg-muted/20 p-6 space-y-4">
            <h3 className="font-semibold text-sm flex items-center gap-2">
              <SparklesIcon className="h-4 w-4 text-primary" /> First-Party Architecture
            </h3>
            <ul className="space-y-3 text-xs text-muted-foreground leading-relaxed">
              <li className="flex items-start gap-2">
                <ArrowRightIcon className="h-3.5 w-3.5 text-primary shrink-0 mt-0.5" />
                <span>
                  <strong className="text-foreground">Zero 3rd-Party SaaS Fees</strong>: Replaces Stape.io and self-hosted SS-GTM Cloud Run containers ($50–$500/mo).
                </span>
              </li>
              <li className="flex items-start gap-2">
                <ArrowRightIcon className="h-3.5 w-3.5 text-primary shrink-0 mt-0.5" />
                <span>
                  <strong className="text-foreground">Browser Deduplication</strong>: Seamlessly reconciles frontend <code className="font-mono text-primary">fbq</code> Pixel and server action events using matching <code className="font-mono text-primary">event_id</code>.
                </span>
              </li>
              <li className="flex items-start gap-2">
                <ArrowRightIcon className="h-3.5 w-3.5 text-primary shrink-0 mt-0.5" />
                <span>
                  <strong className="text-foreground">Ad-Blocker Proof</strong>: Ingested server-to-server with first-party cookies (<code className="font-mono text-primary">_fbp</code>, <code className="font-mono text-primary">_fbc</code>) directly from storefront server actions.
                </span>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
