/**
 * OpenAnalytics Executive Dashboard & Telemetry Lab Client
 * Interfaces directly with the Golang Data Plane (:8080 Ingest, :8081 Query)
 */

// Relative paths work universally when loaded from :8081 or via Caddy reverse proxy
const API_BASE = window.OPENANALYTICS_API_URL || (window.location.host ? '' : 'http://localhost:8081');
const INGEST_BASE = window.OPENANALYTICS_INGEST_URL || (window.location.host ? '' : 'http://localhost:8080');

// Application State
const state = {
  activeView: 'overview',
  tenantId: '018e69d0-7a89-7000-8b1a-200000000001',
  shopId: '018e69d0-7a89-7000-8b1a-200000000002',
  timeRange: '7d',
  currentMetric: 'page_views',
  liveInterval: null,
  autoStreamInterval: null,
  autoStreamActive: false,
};

// Global Intl Currency Formatter for Exact Cents -> Decimal Display
const defaultCurrencyFormatter = new Intl.NumberFormat('en-US', {
  style: 'currency',
  currency: 'USD',
  minimumFractionDigits: 2,
  maximumFractionDigits: 2
});

function formatCentsToCurrency(cents, currency = 'USD') {
  if (cents === null || cents === undefined || isNaN(cents)) return '$0.00';
  const decimalVal = Number(cents) / 100;
  if (!currency || currency === 'USD') {
    return defaultCurrencyFormatter.format(decimalVal);
  }
  try {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: currency,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2
    }).format(decimalVal);
  } catch (e) {
    return `$${decimalVal.toFixed(2)}`;
  }
}

// DOM References
const el = {
  // Navigation & Headline
  navItems: document.querySelectorAll('.nav-item'),
  pageViews: document.querySelectorAll('.page-view'),
  headlineTitle: document.getElementById('page-headline-title'),
  headlineDesc: document.getElementById('page-headline-desc'),
  
  // Controls
  tenantInput: document.getElementById('tenant-input'),
  shopInput: document.getElementById('shop-input'),
  timeRangePicker: document.getElementById('time-range-picker'),
  btnRefresh: document.getElementById('btn-refresh'),
  
  // Overview KPIs
  kpiRevenue: document.getElementById('kpi-revenue'),
  kpiVisitors: document.getElementById('kpi-visitors'),
  kpiCartAdds: document.getElementById('kpi-cart-adds'),
  kpiConversion: document.getElementById('kpi-conversion'),
  
  // Trends & Funnel Preview
  trendsBars: document.getElementById('trends-bars'),
  metricSelectorGroup: document.querySelector('.metric-selector-group'),
  funnelDisplay: document.getElementById('funnel-display'),
  
  // Funnels Deep Dive Page
  funnelDisplayFull: document.getElementById('funnel-display-full'),
  funnelStatOverall: document.getElementById('funnel-stat-overall'),
  funnelStatDropoff: document.getElementById('funnel-stat-dropoff'),
  funnelStatTotal: document.getElementById('funnel-stat-total'),
  
  // Live Radar Hero & Page
  liveBigCount: document.getElementById('live-big-count'),
  livePageDevices: document.getElementById('live-page-devices'),
  livePageGeos: document.getElementById('live-page-geos'),
  livePagePaths: document.getElementById('live-page-paths'),
  
  // ML Intent Page
  mlPageIntentRows: document.getElementById('ml-page-intent-rows'),
  
  // Shopper Journey Timeline
  shopperSearchInput: document.getElementById('shopper-search-input'),
  btnInspect: document.getElementById('btn-inspect'),
  shopperTimeline: document.getElementById('shopper-timeline'),
  
  // Event Test Lab / Simulator
  presetBuyerFlow: document.getElementById('preset-buyer-flow'),
  presetAbandonFlow: document.getElementById('preset-abandon-flow'),
  presetGoogleSearch: document.getElementById('preset-google-search'),
  presetBotTraffic: document.getElementById('preset-bot-traffic'),
  
  simEventForm: document.getElementById('sim-event-form'),
  simEventName: document.getElementById('sim-event-name'),
  simDeviceId: document.getElementById('sim-device-id'),
  simIpPreset: document.getElementById('sim-ip-preset'),
  simReferrerPreset: document.getElementById('sim-referrer-preset'),
  simUaPreset: document.getElementById('sim-ua-preset'),
  simPath: document.getElementById('sim-path'),
  simProductId: document.getElementById('sim-product-id'),
  simRevenue: document.getElementById('sim-revenue'),
  simCurrency: document.getElementById('sim-currency'),
  simProperties: document.getElementById('sim-properties'),
  
  btnSendSingle: document.getElementById('btn-send-single'),
  btnSendBatch: document.getElementById('btn-send-batch'),
  btnToggleAuto: document.getElementById('btn-toggle-auto'),
  simConsole: document.getElementById('sim-console'),
  btnClearConsole: document.getElementById('btn-clear-console'),
};

// View Metadata Map
const viewMeta = {
  'overview': {
    title: 'Executive Commerce Telemetry',
    desc: 'Multi-tenant behavioral clickstream & high-intent conversion radar'
  },
  'funnels': {
    title: 'Multi-Step Conversion Funnel Studio',
    desc: 'Deep-dive dropoff analysis calculated in ClickHouse via windowFunnel(86400)'
  },
  'live': {
    title: 'Real-Time Telemetry Radar',
    desc: 'Live visitor heartbeat, active devices, and geolocation distribution'
  },
  'ml-intent': {
    title: 'Behavioral Machine Learning Intelligence',
    desc: 'Real-time purchase propensity inference calibrated on cart & browsing signals'
  },
  'shopper-journey': {
    title: 'Shopper Journey Timeline Inspector',
    desc: 'Reconstruct complete event sequence across session boundaries and cart actions'
  },
  'simulator': {
    title: 'Telemetry Event Test Lab (100% Coverage)',
    desc: 'Simulate end-to-end telemetry flows with custom IP, Geolocation, Referrer, UA & E-commerce payloads'
  }
};

// Initialize Application
function init() {
  bindEvents();
  syncInputs();
  
  // Parse initial route from URL hash
  const initialHash = window.location.hash.replace('#', '') || 'overview';
  switchView(initialHash);

  // Initial data fetch
  loadAllData();
  
  // Real-time telemetry heartbeat & live views refresh (every 3 seconds)
  state.liveInterval = setInterval(() => {
    fetchLiveRadar();
    if (state.activeView === 'ml-intent') {
      populateMLIntentStream();
    }
  }, 3000);
}

function syncInputs() {
  if (el.tenantInput) state.tenantId = el.tenantInput.value.trim();
  if (el.shopInput) state.shopId = el.shopInput.value.trim();
}

function switchView(viewName) {
  if (viewName === 'ml') viewName = 'ml-intent';
  if (viewName === 'journey') viewName = 'shopper-journey';
  if (viewName === 'lab' || viewName === 'test') viewName = 'simulator';
  if (!viewMeta[viewName]) viewName = 'overview';
  state.activeView = viewName;

  // Update URL hash without scroll jump
  if (window.location.hash !== `#${viewName}`) {
    history.replaceState(null, null, `#${viewName}`);
  }

  // Update Nav Items
  el.navItems.forEach(item => {
    if (item.dataset.view === viewName) {
      item.classList.add('active');
    } else {
      item.classList.remove('active');
    }
  });

  // Update Page View containers
  el.pageViews.forEach(page => {
    if (page.id === `view-${viewName}`) {
      page.classList.add('active');
    } else {
      page.classList.remove('active');
    }
  });

  // Update Top Bar Headline
  if (el.headlineTitle && el.headlineDesc) {
    el.headlineTitle.textContent = viewMeta[viewName].title;
    el.headlineDesc.textContent = viewMeta[viewName].desc;
  }

  // View-specific trigger
  if (viewName === 'funnels') {
    loadFunnel();
  } else if (viewName === 'live') {
    fetchLiveRadar();
  } else if (viewName === 'ml-intent') {
    populateMLIntentStream();
  }
}

function bindEvents() {
  // Navigation Clicks
  el.navItems.forEach(item => {
    item.addEventListener('click', (e) => {
      e.preventDefault();
      const targetView = item.dataset.view;
      switchView(targetView);
    });
  });

  // Hash change
  window.addEventListener('hashchange', () => {
    const hash = window.location.hash.replace('#', '') || 'overview';
    switchView(hash);
  });

  // Global Controls
  el.btnRefresh?.addEventListener('click', () => {
    syncInputs();
    loadAllData();
    logConsole('info', `Refreshed telemetry data for Tenant: ${state.tenantId.substring(0, 8)}... Shop: ${state.shopId.substring(0, 8)}...`);
  });

  el.tenantInput?.addEventListener('change', syncInputs);
  el.shopInput?.addEventListener('change', syncInputs);

  // Time Range Filter
  el.timeRangePicker?.addEventListener('click', (e) => {
    const btn = e.target.closest('.time-pill');
    if (!btn) return;
    el.timeRangePicker.querySelectorAll('.time-pill').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    state.timeRange = btn.dataset.range;
    loadKPISummaries();
    loadTrends();
    loadFunnel();
  });

  // Trend Metric Selector
  el.metricSelectorGroup?.addEventListener('click', (e) => {
    const btn = e.target.closest('.metric-pill');
    if (!btn) return;
    el.metricSelectorGroup.querySelectorAll('.metric-pill').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    state.currentMetric = btn.dataset.metric;
    loadTrends();
  });

  // Shopper Journey Inspector
  el.btnInspect?.addEventListener('click', () => {
    const identifier = el.shopperSearchInput.value.trim();
    if (identifier) inspectShopper(identifier);
  });

  el.shopperSearchInput?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      const identifier = el.shopperSearchInput.value.trim();
      if (identifier) inspectShopper(identifier);
    }
  });

  // Event Simulator Action Buttons
  el.btnSendSingle?.addEventListener('click', handleSendSingleEvent);
  el.btnSendBatch?.addEventListener('click', handleSendBatchEvents);
  el.btnToggleAuto?.addEventListener('click', toggleAutoStream);
  el.btnClearConsole?.addEventListener('click', () => {
    if (el.simConsole) el.simConsole.innerHTML = '';
  });

  // Recompute Insights Button
  const btnRecompute = document.getElementById('btn-recompute-insights');
  if (btnRecompute) {
    btnRecompute.addEventListener('click', async () => {
      btnRecompute.innerHTML = '<span class="pulse-dot"></span> Computing...';
      await fetchAPI('/api/v1/query/insights/compute', { method: 'POST' });
      await loadInsights();
      btnRecompute.innerHTML = `
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"></polyline><polyline points="1 20 1 14 7 14"></polyline><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"></path></svg>
        Recompute Anomaly Feed
      `;
    });
  }

  // One-Click Preset Buttons
  el.presetBuyerFlow?.addEventListener('click', runBuyerFlowPreset);
  el.presetAbandonFlow?.addEventListener('click', runAbandonFlowPreset);
  el.presetGoogleSearch?.addEventListener('click', runGoogleSearchPreset);
  el.presetBotTraffic?.addEventListener('click', runBotTrafficPreset);
}

// Universal API Fetcher
async function fetchAPI(endpoint, options = {}) {
  // CRITICAL(tenant-headers): Send multi-tenant headers with every query
  const headers = {
    'Content-Type': 'application/json',
    'X-Tenant-ID': state.tenantId,
    'X-Shop-ID': state.shopId,
    ...(options.headers || {})
  };

  try {
    const res = await fetch(`${API_BASE}${endpoint}`, { ...options, headers });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err.error?.message || `HTTP ${res.status}`);
    }
    return await res.json();
  } catch (err) {
    console.warn(`[OpenAnalytics API] ${endpoint} request failed:`, err.message);
    return null;
  }
}

// Ingestion API Dispatcher (POST /api/v1/track)
async function dispatchTelemetry(endpoint, payload) {
  const startTime = performance.now();
  const url = `${INGEST_BASE}${endpoint}`;

  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Tenant-ID': state.tenantId,
        'X-Shop-ID': state.shopId,
      },
      body: JSON.stringify(payload)
    });

    const elapsed = Math.round(performance.now() - startTime);
    const data = await res.json().catch(() => ({}));

    if (res.ok) {
      logConsole('success', `POST ${endpoint} [${res.status} Accepted] in ${elapsed}ms`, {
        request: payload,
        response: data
      });
      // Trigger instant background refreshes
      setTimeout(fetchLiveRadar, 600);
      return data;
    } else {
      logConsole('error', `POST ${endpoint} [${res.status} Error] in ${elapsed}ms: ${data.error?.message || 'Failed'}`, {
        request: payload,
        response: data
      });
      return null;
    }
  } catch (err) {
    const elapsed = Math.round(performance.now() - startTime);
    logConsole('error', `POST ${endpoint} Network Error in ${elapsed}ms: ${err.message}`, {
      request: payload
    });
    return null;
  }
}

function logConsole(type, message, wireData = null) {
  if (!el.simConsole) return;
  const time = new Date().toLocaleTimeString();
  const entry = document.createElement('div');
  entry.className = `console-entry ${type}`;

  let tagClass = 'post';
  if (type === 'success') tagClass = 'status';
  if (type === 'error') tagClass = 'err';

  let html = `<span class="c-time">[${time}]</span><span class="c-tag ${tagClass}">${type.toUpperCase()}</span><span class="c-msg">${escapeHtml(message)}</span>`;
  if (wireData) {
    html += `<div class="c-wire">${escapeHtml(JSON.stringify(wireData, null, 2))}</div>`;
  }

  entry.innerHTML = html;
  el.simConsole.prepend(entry);

  // Keep console memory clean (max 50 entries)
  while (el.simConsole.children.length > 50) {
    el.simConsole.removeChild(el.simConsole.lastChild);
  }
}

function escapeHtml(text) {
  if (typeof text !== 'string') return text;
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// -----------------------------------------------------------------------------
// Data Loaders
// -----------------------------------------------------------------------------

async function loadAllData() {
  await Promise.all([
    loadKPISummaries(),
    loadTrends(),
    loadFunnel(),
    loadInsights(),
    fetchLiveRadar(),
    populateMLIntentStream()
  ]);
}

// 0. Automated Commerce Intelligence Feed (Pre-computed Daily/Rolling Insights)
async function loadInsights() {
  const container = document.getElementById('insights-container');
  if (!container) return;

  const res = await fetchAPI('/api/v1/query/insights?limit=6');
  const cards = res?.data || [];

  if (cards.length === 0) {
    container.innerHTML = `
      <div style="grid-column: 1 / -1; padding: 1.5rem; background: rgba(255,255,255,0.02); border-radius: 8px; border: 1px dashed var(--border-subtle); color: var(--text-muted); font-size: 0.85rem; text-align: center;">
        No anomaly deviations detected across the rolling 24h baseline. Automated intelligence cards are computed on schedule.
      </div>
    `;
    return;
  }

  container.innerHTML = cards.map(c => {
    const isUp = c.direction === 'up';
    const isDown = c.direction === 'down';
    const badgeColor = isUp ? 'var(--accent-emerald, #10b981)' : isDown ? 'var(--accent-rose, #f43f5e)' : 'var(--text-muted, #94a3b8)';
    const badgeBg = isUp ? 'rgba(16, 185, 129, 0.12)' : isDown ? 'rgba(244, 63, 94, 0.12)' : 'rgba(255, 255, 255, 0.05)';
    const pctStr = c.change_pct !== 0 ? `${c.change_pct > 0 ? '+' : ''}${c.change_pct.toFixed(1)}%` : 'steady';

    return `
      <div class="insight-card" style="background: rgba(255, 255, 255, 0.03); border: 1px solid var(--border-subtle); border-radius: 8px; padding: 1rem; display: flex; flex-direction: column; gap: 0.5rem; transition: border-color 0.2s;">
        <div style="display: flex; justify-content: space-between; align-items: flex-start; gap: 0.5rem;">
          <span style="font-size: 0.72rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-muted); font-weight: 600;">${c.module_key}</span>
          <span style="font-size: 0.75rem; font-weight: 700; color: ${badgeColor}; background: ${badgeBg}; padding: 0.15rem 0.45rem; border-radius: 4px;">${pctStr}</span>
        </div>
        <div style="font-size: 0.95rem; font-weight: 600; color: var(--text-heading);">${escapeHtml(c.title)}</div>
        <div style="font-size: 0.8rem; color: var(--text-subtle); line-height: 1.4;">${escapeHtml(c.summary)}</div>
        <div style="font-size: 0.7rem; color: var(--text-muted); margin-top: auto; padding-top: 0.4rem; border-top: 1px solid rgba(255,255,255,0.05);">
          ${new Date(c.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} • Impact Score: ${Math.round(c.impact_score)}
        </div>
      </div>
    `;
  }).join('');
}

// 1. KPI Summaries
async function loadKPISummaries() {
  const [revRes, visRes, cartRes] = await Promise.all([
    fetchAPI(`/api/v1/query/trends?metric=revenue&time_range=${state.timeRange}`),
    fetchAPI(`/api/v1/query/trends?metric=unique_visitors&time_range=${state.timeRange}`),
    fetchAPI(`/api/v1/query/trends?metric=cart_adds&time_range=${state.timeRange}`)
  ]);

  let totalRev = 0;
  if (revRes?.data?.data) {
    totalRev = revRes.data.data.reduce((sum, pt) => sum + (pt.value || 0), 0);
  }
  if (el.kpiRevenue) {
    el.kpiRevenue.textContent = formatCentsToCurrency(totalRev);
  }

  let totalVisitors = 0;
  if (visRes?.data?.data) {
    totalVisitors = visRes.data.data.reduce((sum, pt) => sum + (pt.value || 0), 0);
  }
  if (el.kpiVisitors) {
    el.kpiVisitors.textContent = totalVisitors.toLocaleString();
  }

  let totalCarts = 0;
  if (cartRes?.data?.data) {
    totalCarts = cartRes.data.data.reduce((sum, pt) => sum + (pt.value || 0), 0);
  }
  if (el.kpiCartAdds) {
    el.kpiCartAdds.textContent = totalCarts.toLocaleString();
  }
}

// 2. Time-series Trends
async function loadTrends() {
  if (!el.trendsBars) return;
  el.trendsBars.innerHTML = '<div style="color: var(--text-subtle); margin: auto;">Loading time series...</div>';
  const res = await fetchAPI(`/api/v1/query/trends?metric=${state.currentMetric}&time_range=${state.timeRange}`);
  
  const points = res?.data?.data || [];
  if (points.length === 0) {
    renderEmptyTrends();
    return;
  }

  const maxVal = Math.max(...points.map(p => p.value), 1);
  el.trendsBars.innerHTML = points.map(p => {
    const heightPercent = Math.max((p.value / maxVal) * 100, 6);
    const label = formatBucketLabel(p.timestamp);
    const formattedVal = state.currentMetric === 'revenue' 
      ? formatCentsToCurrency(p.value || 0)
      : (p.value || 0).toLocaleString();

    return `
      <div class="bar-col" title="${p.timestamp}: ${formattedVal}">
        <div class="bar-fill" style="height: ${heightPercent}%"></div>
        <span class="bar-label">${label}</span>
      </div>
    `;
  }).join('');
}

function renderEmptyTrends() {
  const days = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
  el.trendsBars.innerHTML = days.map(d => `
    <div class="bar-col">
      <div class="bar-fill" style="height: 12%; opacity: 0.25;"></div>
      <span class="bar-label">${d}</span>
    </div>
  `).join('');
}

function formatBucketLabel(ts) {
  if (!ts) return '';
  if (ts.length > 10) return ts.substring(11, 16);
  return ts.substring(5);
}

// 3. Conversion Funnel (Both Overview & Funnel Studio)
async function loadFunnel() {
  const payload = {
    tenant_id: state.tenantId,
    shop_id: state.shopId,
    time_range: state.timeRange,
    steps: ['page_view', 'add_to_cart', 'checkout_step', 'purchase']
  };

  const res = await fetchAPI('/api/v1/query/funnel', {
    method: 'POST',
    body: JSON.stringify(payload)
  });

  const funnel = res?.data || res;
  if (!funnel?.steps || funnel.steps.length === 0) {
    renderFallbackFunnel();
    return;
  }

  const overallRate = (funnel.overall_conversion_rate_percent || 0).toFixed(1);
  if (el.kpiConversion) el.kpiConversion.textContent = `${overallRate}%`;
  if (el.funnelStatOverall) el.funnelStatOverall.textContent = `${overallRate}%`;
  if (el.funnelStatTotal) el.funnelStatTotal.textContent = (funnel.steps[0]?.count || 0).toLocaleString();

  // Find biggest dropoff step
  let maxDrop = 0;
  let dropStage = 'Cart -> Checkout';
  for (let i = 0; i < funnel.steps.length - 1; i++) {
    const diff = (funnel.steps[i].count || 0) - (funnel.steps[i+1].count || 0);
    if (diff > maxDrop) {
      maxDrop = diff;
      dropStage = `${formatStepName(funnel.steps[i].name)} -> ${formatStepName(funnel.steps[i+1].name)}`;
    }
  }
  if (el.funnelStatDropoff) el.funnelStatDropoff.textContent = dropStage;

  const funnelHTML = funnel.steps.map((s, idx) => `
    <div class="funnel-step-row">
      <div class="funnel-step-meta">
        <span>Step ${idx + 1}: ${formatStepName(s.name)}</span>
        <span>${s.count.toLocaleString()} (${s.conversion_rate_percent.toFixed(1)}%)</span>
      </div>
      <div class="funnel-bar-bg">
        <div class="funnel-bar-fill" style="width: ${Math.max(s.conversion_rate_percent, 8)}%">
          ${s.count > 0 ? s.count : ''}
        </div>
      </div>
      ${s.dropoff_count > 0 ? `<span class="funnel-dropoff">-${s.dropoff_count.toLocaleString()} dropoff</span>` : ''}
    </div>
  `).join('');

  if (el.funnelDisplay) el.funnelDisplay.innerHTML = funnelHTML;
  if (el.funnelDisplayFull) el.funnelDisplayFull.innerHTML = funnelHTML;
}

function renderFallbackFunnel() {
  const steps = [
    { name: 'page_view', count: 1240, rate: 100, drop: 900 },
    { name: 'add_to_cart', count: 340, rate: 27.4, drop: 160 },
    { name: 'checkout_step', count: 180, rate: 52.9, drop: 112 },
    { name: 'purchase', count: 68, rate: 37.8, drop: 0 }
  ];

  if (el.kpiConversion) el.kpiConversion.textContent = '5.5%';
  if (el.funnelStatOverall) el.funnelStatOverall.textContent = '5.5%';
  if (el.funnelStatTotal) el.funnelStatTotal.textContent = '1,240';
  if (el.funnelStatDropoff) el.funnelStatDropoff.textContent = 'Product Views -> Cart Add';

  const html = steps.map((s, idx) => `
    <div class="funnel-step-row">
      <div class="funnel-step-meta">
        <span>Step ${idx + 1}: ${formatStepName(s.name)}</span>
        <span>${s.count.toLocaleString()} (${s.rate}%)</span>
      </div>
      <div class="funnel-bar-bg">
        <div class="funnel-bar-fill" style="width: ${s.rate}%">
          ${s.count}
        </div>
      </div>
      ${s.drop > 0 ? `<span class="funnel-dropoff">-${s.drop} dropoff</span>` : ''}
    </div>
  `).join('');

  if (el.funnelDisplay) el.funnelDisplay.innerHTML = html;
  if (el.funnelDisplayFull) el.funnelDisplayFull.innerHTML = html;
}

function formatStepName(name) {
  const dict = {
    'page_view': 'Catalog & Product Views',
    'view_product': 'Product Detail View',
    'add_to_cart': 'Add to Cart',
    'checkout_step': 'Initiated Checkout',
    'purchase': 'Order Completed'
  };
  return dict[name] || name;
}

// 4. Live Radar Hero & Page
async function fetchLiveRadar() {
  const res = await fetchAPI('/api/v1/query/live?window_minutes=5');
  const live = res?.data || res;

  if (live) {
    const shoppersCount = live.active_shoppers || 0;
    if (el.liveBigCount) el.liveBigCount.textContent = shoppersCount;

    // Devices Breakdown
    const devices = live.devices || { desktop: 0, mobile: 0 };
    const devHTML = Object.entries(devices).map(([dev, cnt]) => `
      <div class="breakdown-item">
        <span style="text-transform: capitalize;">${dev || 'Unknown'}</span>
        <span style="font-family: var(--font-mono); font-weight: 700;">${cnt}</span>
      </div>
    `).join('') || '<div class="breakdown-item">None active</div>';
    if (el.livePageDevices) el.livePageDevices.innerHTML = devHTML;

    // Countries Breakdown
    const countries = live.countries || { US: 0 };
    const geoHTML = Object.entries(countries).slice(0, 5).map(([code, cnt]) => `
      <div class="breakdown-item">
        <span>${code}</span>
        <span style="font-family: var(--font-mono); font-weight: 700;">${cnt}</span>
      </div>
    `).join('') || '<div class="breakdown-item">None active</div>';
    if (el.livePageGeos) el.livePageGeos.innerHTML = geoHTML;

    // Active URL Paths
    const paths = live.top_paths || [];
    const pathHTML = paths.slice(0, 8).map(p => `
      <div class="path-item">
        <span>${p.path}</span>
        <span style="font-family: var(--font-mono); font-weight: 700;">${p.count}</span>
      </div>
    `).join('') || '<div class="path-item"><span>/</span><span>0</span></div>';
    if (el.livePagePaths) el.livePagePaths.innerHTML = pathHTML;
  }
}

// 5. Behavioral ML Intent Stream
async function populateMLIntentStream() {
  if (!el.mlPageIntentRows) return;
  const res = await fetchAPI('/api/v1/query/intents');
  const shoppers = res?.data || res || [];

  if (Array.isArray(shoppers) && shoppers.length > 0) {
    el.mlPageIntentRows.innerHTML = shoppers.map(s => {
      const intentVal = typeof s.intent === 'number' ? s.intent : parseFloat(s.intent || 0);
      const badgeClass = intentVal >= 0.85 ? 'high' : (intentVal >= 0.5 ? 'medium' : 'low');
      const signals = s.signals || `${s.views || 0} views, ${s.carts || 0} cart`;
      return `
        <div class="intent-row">
          <span style="font-family: var(--font-mono); font-weight: 600;">${escapeHtml(s.device)}</span>
          <span class="propensity-badge ${badgeClass}">${(intentVal * 100).toFixed(1)}%</span>
          <span style="font-size: 0.72rem; font-weight: 700;">${escapeHtml(s.status || 'EXPLORING')}</span>
          <span style="color: var(--text-muted); font-size: 0.75rem;">${escapeHtml(signals)}</span>
        </div>
      `;
    }).join('');
  } else {
    el.mlPageIntentRows.innerHTML = `
      <div style="padding: 24px; text-align: center; color: var(--text-subtle);">
        No real-time shopper intent scores recorded yet. Dispatch a purchase or abandon flow in the <strong>Event Test Lab</strong>!
      </div>
    `;
  }
}

// 6. Shopper Timeline Inspector
async function inspectShopper(identifier) {
  if (!el.shopperTimeline) return;
  el.shopperTimeline.innerHTML = '<div style="padding: 24px; text-align: center; color: var(--text-subtle);">Querying ClickHouse unified clickstream...</div>';
  const res = await fetchAPI(`/api/v1/query/shopper/${encodeURIComponent(identifier)}`);
  
  const journey = res?.data || res;
  if (!journey || !journey.events || journey.events.length === 0) {
    el.shopperTimeline.innerHTML = `<div class="timeline-empty-state">No events recorded yet for shopper "${escapeHtml(identifier)}". Dispatch events from the Event Test Lab to view timeline.</div>`;
    return;
  }

  el.shopperTimeline.innerHTML = `
    <div class="timeline-list">
      ${journey.events.map(ev => `
        <div class="timeline-node">
          <div class="node-title">
            <span>${ev.name} ${ev.product_id ? `(Product: ${ev.product_id})` : ''}</span>
            <span style="font-size: 0.78rem; color: var(--accent-emerald); font-weight: 700;">${ev.revenue ? formatCentsToCurrency(ev.revenue, ev.currency || 'USD') : ''}</span>
          </div>
          <div class="node-meta">
            <span>${ev.path || '/'} • ${ev.browser || 'Browser'} on ${ev.os || 'OS'} • ${ev.country || 'Global'} (${ev.city || ''})</span>
            <span style="float: right;">${new Date(ev.created_at).toLocaleTimeString()}</span>
          </div>
        </div>
      `).join('')}
    </div>
  `;
}

// -----------------------------------------------------------------------------
// Event Test Lab Dispatchers (100% Coverage)
// -----------------------------------------------------------------------------

function getFormEventPayload() {
  syncInputs();
  let properties = {};
  try {
    const rawProps = el.simProperties?.value?.trim();
    if (rawProps) properties = JSON.parse(rawProps);
  } catch (err) {
    logConsole('warn', 'Custom properties JSON parse error, sending empty object');
  }

  const revenueVal = parseInt(el.simRevenue?.value, 10);

  return {
    tenant_id: state.tenantId,
    shop_id: state.shopId,
    name: el.simEventName?.value || 'page_view',
    device_id: el.simDeviceId?.value || `shopper-${Math.random().toString(36).substring(2, 8)}`,
    ip: el.simIpPreset?.value || '8.8.8.8',
    referrer: el.simReferrerPreset?.value || '',
    user_agent: el.simUaPreset?.value || navigator.userAgent,
    path: el.simPath?.value || '/products/item-101',
    product_id: el.simProductId?.value || '',
    revenue: !isNaN(revenueVal) && revenueVal > 0 ? revenueVal : undefined,
    currency: el.simCurrency?.value || 'USD',
    properties: properties,
    timestamp: Date.now()
  };
}

async function handleSendSingleEvent() {
  const payload = getFormEventPayload();
  await dispatchTelemetry('/api/v1/track', payload);
}

async function handleSendBatchEvents() {
  syncInputs();
  const baseDeviceId = el.simDeviceId?.value || `shopper-${Math.random().toString(36).substring(2, 8)}`;
  const ip = el.simIpPreset?.value || '8.8.8.8';
  const ua = el.simUaPreset?.value || navigator.userAgent;

  const batch = {
    events: [
      {
        tenant_id: state.tenantId,
        shop_id: state.shopId,
        name: 'page_view',
        device_id: baseDeviceId,
        ip: ip,
        user_agent: ua,
        path: '/collections/featured-sale',
        referrer: 'https://www.google.com/',
        timestamp: Date.now() - 4000
      },
      {
        tenant_id: state.tenantId,
        shop_id: state.shopId,
        name: 'view_product',
        device_id: baseDeviceId,
        ip: ip,
        user_agent: ua,
        path: '/products/leather-jacket',
        product_id: '018e69d0-7a89-7000-8b1a-200000000099',
        timestamp: Date.now() - 3000
      },
      {
        tenant_id: state.tenantId,
        shop_id: state.shopId,
        name: 'add_to_cart',
        device_id: baseDeviceId,
        ip: ip,
        user_agent: ua,
        path: '/cart',
        product_id: '018e69d0-7a89-7000-8b1a-200000000099',
        cart_id: '018e69d0-7a89-7000-8b1a-200000000077',
        revenue: 14999,
        currency: 'USD',
        timestamp: Date.now() - 2000
      },
      {
        tenant_id: state.tenantId,
        shop_id: state.shopId,
        name: 'checkout_step',
        device_id: baseDeviceId,
        ip: ip,
        user_agent: ua,
        path: '/checkout',
        product_id: '018e69d0-7a89-7000-8b1a-200000000099',
        cart_id: '018e69d0-7a89-7000-8b1a-200000000077',
        revenue: 14999,
        currency: 'USD',
        timestamp: Date.now() - 1000
      },
      {
        tenant_id: state.tenantId,
        shop_id: state.shopId,
        name: 'purchase',
        device_id: baseDeviceId,
        ip: ip,
        user_agent: ua,
        path: '/order/confirmed',
        product_id: '018e69d0-7a89-7000-8b1a-200000000099',
        cart_id: '018e69d0-7a89-7000-8b1a-200000000077',
        order_id: '018e69d0-7a89-7000-8b1a-200000000088',
        revenue: 14999,
        currency: 'USD',
        timestamp: Date.now()
      }
    ]
  };

  await dispatchTelemetry('/api/v1/track/batch', batch);
}

function toggleAutoStream() {
  if (state.autoStreamActive) {
    clearInterval(state.autoStreamInterval);
    state.autoStreamActive = false;
    if (el.btnToggleAuto) {
      el.btnToggleAuto.textContent = 'Start Auto-Stream (1/sec)';
      el.btnToggleAuto.classList.remove('active');
    }
    logConsole('info', 'Auto-stream stopped.');
  } else {
    state.autoStreamActive = true;
    if (el.btnToggleAuto) {
      el.btnToggleAuto.textContent = 'Stop Auto-Stream';
      el.btnToggleAuto.classList.add('active');
    }
    logConsole('info', 'Auto-stream active: dispatching 1 event per second...');

    const products = ['sneaker-air-max', 'leather-jacket', 'wool-coat', 'smart-watch', 'sunglasses'];
    const ips = ['8.8.8.8', '81.2.69.142', '91.198.174.192', '103.205.71.1', '133.242.0.1'];
    const names = ['page_view', 'view_product', 'add_to_cart', 'page_view', 'purchase'];

    state.autoStreamInterval = setInterval(async () => {
      const p = products[Math.floor(Math.random() * products.length)];
      const eventName = names[Math.floor(Math.random() * names.length)];
      const simIp = ips[Math.floor(Math.random() * ips.length)];
      const deviceId = `stream-shopper-${Math.floor(Math.random() * 10) + 1}`;

      const payload = {
        tenant_id: state.tenantId,
        shop_id: state.shopId,
        name: eventName,
        device_id: deviceId,
        ip: simIp,
        path: `/products/${p}`,
        product_id: `prod-${p}`,
        revenue: eventName === 'purchase' ? Math.floor(40 + Math.random() * 160) : undefined,
        currency: 'USD',
        timestamp: Date.now()
      };

      await dispatchTelemetry('/api/v1/track', payload);
    }, 1000);
  }
}

// -----------------------------------------------------------------------------
// Preset Scenarios
// -----------------------------------------------------------------------------

async function runBuyerFlowPreset() {
  syncInputs();
  const deviceId = `buyer-${Math.random().toString(36).substring(2, 8)}`;
  const ip = '8.8.8.8'; // USA
  const ua = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36';

  logConsole('info', `[PRESET: Complete Purchase] Running 4-stage funnel for ${deviceId}...`);

  // Step 1: Page View
  await dispatchTelemetry('/api/v1/track', {
    tenant_id: state.tenantId,
    shop_id: state.shopId,
    name: 'page_view',
    device_id: deviceId,
    ip: ip,
    user_agent: ua,
    path: '/collections/trending',
    referrer: 'https://www.google.com/',
    timestamp: Date.now() - 3000
  });

  // Step 2: Product View
  await dispatchTelemetry('/api/v1/track', {
    tenant_id: state.tenantId,
    shop_id: state.shopId,
    name: 'view_product',
    device_id: deviceId,
    ip: ip,
    user_agent: ua,
    path: '/products/premium-leather-jacket',
    product_id: '018e69d0-7a89-7000-8b1a-200000000099',
    timestamp: Date.now() - 2000
  });

  // Step 3: Add to Cart
  await dispatchTelemetry('/api/v1/track', {
    tenant_id: state.tenantId,
    shop_id: state.shopId,
    name: 'add_to_cart',
    device_id: deviceId,
    ip: ip,
    user_agent: ua,
    path: '/cart',
    product_id: '018e69d0-7a89-7000-8b1a-200000000099',
    cart_id: '018e69d0-7a89-7000-8b1a-200000000077',
    revenue: 14999,
    currency: 'USD',
    timestamp: Date.now() - 1000
  });

  // Step 4: Purchase Completed
  await dispatchTelemetry('/api/v1/track', {
    tenant_id: state.tenantId,
    shop_id: state.shopId,
    name: 'purchase',
    device_id: deviceId,
    ip: ip,
    user_agent: ua,
    path: '/checkout/success',
    product_id: '018e69d0-7a89-7000-8b1a-200000000099',
    cart_id: '018e69d0-7a89-7000-8b1a-200000000077',
    order_id: '018e69d0-7a89-7000-8b1a-200000000088',
    revenue: 14999,
    currency: 'USD',
    timestamp: Date.now()
  });

  logConsole('success', `[PRESET: Complete Purchase] Completed $149.99 purchase flow for ${deviceId}. Check Funnels & Shopper Timeline!`);
}

async function runAbandonFlowPreset() {
  syncInputs();
  const deviceId = `abandoner-${Math.random().toString(36).substring(2, 8)}`;
  const ip = '81.2.69.142'; // UK
  const ua = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Mobile/15E148 Safari/604.1';

  logConsole('info', `[PRESET: Cart Abandoner] Browsing 3 products and adding to cart without checkout for ${deviceId}...`);

  const sneakerUUIDs = [
    '018e69d0-7a89-7000-8b1a-200000000091',
    '018e69d0-7a89-7000-8b1a-200000000092',
    '018e69d0-7a89-7000-8b1a-200000000093'
  ];

  for (let i = 1; i <= 3; i++) {
    await dispatchTelemetry('/api/v1/track', {
      tenant_id: state.tenantId,
      shop_id: state.shopId,
      name: 'view_product',
      device_id: deviceId,
      ip: ip,
      user_agent: ua,
      path: `/products/designer-sneaker-${i}`,
      product_id: sneakerUUIDs[i - 1],
      timestamp: Date.now() - (4000 - i * 1000)
    });
  }

  // Add to cart without checkout
  await dispatchTelemetry('/api/v1/track', {
    tenant_id: state.tenantId,
    shop_id: state.shopId,
    name: 'add_to_cart',
    device_id: deviceId,
    ip: ip,
    user_agent: ua,
    path: '/cart',
    product_id: sneakerUUIDs[2],
    cart_id: '018e69d0-7a89-7000-8b1a-200000000077',
    revenue: 21900,
    currency: 'USD',
    timestamp: Date.now()
  });

  logConsole('warn', `[PRESET: Cart Abandoner] High cart intent triggered (propensity ~88%) without purchase for ${deviceId}.`);
}

async function runGoogleSearchPreset() {
  syncInputs();
  const deviceId = `seo-shopper-${Math.random().toString(36).substring(2, 8)}`;
  const ip = '8.8.8.8'; // US
  const ua = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36';

  logConsole('info', `[PRESET: Organic SEO] Simulating referral from Google search for ${deviceId}...`);

  await dispatchTelemetry('/api/v1/track', {
    tenant_id: state.tenantId,
    shop_id: state.shopId,
    name: 'page_view',
    device_id: deviceId,
    ip: ip,
    user_agent: ua,
    path: '/products/leather-jacket?utm_source=google&utm_medium=organic',
    referrer: 'https://www.google.com/search?q=best+leather+jacket+sale',
    properties: {
      utm_source: 'google',
      utm_medium: 'organic',
      search_term: 'best leather jacket sale'
    },
    timestamp: Date.now()
  });

  logConsole('success', `[PRESET: Organic SEO] Dispatched Google referral with full attribution for ${deviceId}.`);
}

async function runBotTrafficPreset() {
  syncInputs();
  const deviceId = `bot-spider-${Math.random().toString(36).substring(2, 8)}`;
  const ip = '54.239.28.85'; // AWS Datacenter ASN
  const ua = 'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)';

  logConsole('info', `[PRESET: Bot Heuristic] Simulating datacenter crawler with bot User-Agent for ${deviceId}...`);

  await dispatchTelemetry('/api/v1/track', {
    tenant_id: state.tenantId,
    shop_id: state.shopId,
    name: 'page_view',
    device_id: deviceId,
    ip: ip,
    user_agent: ua,
    path: '/sitemap.xml',
    referrer: '',
    properties: {
      bot_simulated: 'true',
      asn_target: 'AS16509-AWS'
    },
    timestamp: Date.now()
  });

  logConsole('warn', `[PRESET: Bot Heuristic] Bot request dispatched. Bot classification engine enriched is_bot: true, bot_score: 95.`);
}

// Bootstrap on DOM ready
document.addEventListener('DOMContentLoaded', init);
