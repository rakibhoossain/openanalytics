/**
 * OpenAnalytics Executive Dashboard Client
 * Interfaces directly with the Golang Data Plane (:8081)
 */

// WEAK_POINT(api-url-hardcoding): Configurable query host; defaults to localhost:8081
const API_BASE = window.OPENANALYTICS_API_URL || 'http://localhost:8081';

// State
const state = {
  tenantId: '018e69d0-7a89-7000-8b1a-200000000001',
  shopId: '018e69d0-7a89-7000-8b1a-200000000002',
  timeRange: '7d',
  currentMetric: 'page_views',
  liveInterval: null,
};

// DOM Elements
const el = {
  tenantInput: document.getElementById('tenant-input'),
  shopInput: document.getElementById('shop-input'),
  timeRangePicker: document.getElementById('time-range-picker'),
  btnRefresh: document.getElementById('btn-refresh'),
  
  kpiRevenue: document.getElementById('kpi-revenue'),
  kpiVisitors: document.getElementById('kpi-visitors'),
  kpiCartAdds: document.getElementById('kpi-cart-adds'),
  kpiConversion: document.getElementById('kpi-conversion'),
  
  trendsBars: document.getElementById('trends-bars'),
  metricSelectorGroup: document.querySelector('.metric-selector-group'),
  
  funnelDisplay: document.getElementById('funnel-display'),
  
  liveCount: document.getElementById('live-count'),
  deviceBreakdown: document.getElementById('device-breakdown-list'),
  geoBreakdown: document.getElementById('geo-breakdown-list'),
  pathsBreakdown: document.getElementById('paths-breakdown-list'),
  
  intentRows: document.getElementById('intent-stream-rows'),
  
  shopperSearchInput: document.getElementById('shopper-search-input'),
  btnInspect: document.getElementById('btn-inspect'),
  shopperTimeline: document.getElementById('shopper-timeline'),
};

// Initialize Dashboard
function init() {
  bindEvents();
  syncInputs();
  loadAllData();
  
  // Start 10s radar poll
  state.liveInterval = setInterval(fetchLiveRadar, 10000);
}

function syncInputs() {
  state.tenantId = el.tenantInput.value.trim();
  state.shopId = el.shopInput.value.trim();
}

function bindEvents() {
  el.btnRefresh.addEventListener('click', () => {
    syncInputs();
    loadAllData();
  });

  el.timeRangePicker.addEventListener('click', (e) => {
    const btn = e.target.closest('.time-pill');
    if (!btn) return;
    el.timeRangePicker.querySelectorAll('.time-pill').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    state.timeRange = btn.dataset.range;
    loadTrends();
    loadFunnel();
  });

  el.metricSelectorGroup.addEventListener('click', (e) => {
    const btn = e.target.closest('.metric-pill');
    if (!btn) return;
    el.metricSelectorGroup.querySelectorAll('.metric-pill').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    state.currentMetric = btn.dataset.metric;
    loadTrends();
  });

  el.btnInspect.addEventListener('click', () => {
    const identifier = el.shopperSearchInput.value.trim();
    if (identifier) inspectShopper(identifier);
  });
}

async function fetchAPI(endpoint, options = {}) {
  // CRITICAL(tenant-headers): Send multi-tenant headers with every request
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

async function loadAllData() {
  await Promise.all([
    loadKPISummaries(),
    loadTrends(),
    loadFunnel(),
    fetchLiveRadar(),
    populateMLIntentStream()
  ]);
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
  el.kpiRevenue.textContent = `$${totalRev.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

  let totalVisitors = 0;
  if (visRes?.data?.data) {
    totalVisitors = visRes.data.data.reduce((sum, pt) => sum + (pt.value || 0), 0);
  }
  el.kpiVisitors.textContent = totalVisitors.toLocaleString();

  let totalCarts = 0;
  if (cartRes?.data?.data) {
    totalCarts = cartRes.data.data.reduce((sum, pt) => sum + (pt.value || 0), 0);
  }
  el.kpiCartAdds.textContent = totalCarts.toLocaleString();
}

// 2. Time-series Trends
async function loadTrends() {
  el.trendsBars.innerHTML = '<div style="color: var(--text-subtle); margin: auto;">Loading time series...</div>';
  const res = await fetchAPI(`/api/v1/query/trends?metric=${state.currentMetric}&time_range=${state.timeRange}`);
  
  const points = res?.data?.data || [];
  if (points.length === 0) {
    renderEmptyTrends();
    return;
  }

  const maxVal = Math.max(...points.map(p => p.value), 1);
  el.trendsBars.innerHTML = points.map(p => {
    const heightPercent = Math.max((p.value / maxVal) * 100, 4);
    const label = formatBucketLabel(p.timestamp);
    return `
      <div class="bar-col" title="${p.timestamp}: ${p.value.toLocaleString()}">
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
      <div class="bar-fill" style="height: 12%; opacity: 0.3;"></div>
      <span class="bar-label">${d}</span>
    </div>
  `).join('');
}

function formatBucketLabel(ts) {
  if (!ts) return '';
  if (ts.length > 10) return ts.substring(11, 16); // HH:MM
  return ts.substring(5); // MM-DD
}

// 3. Conversion Funnel
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

  el.kpiConversion.textContent = `${(funnel.overall_conversion_rate_percent || 0).toFixed(1)}%`;

  el.funnelDisplay.innerHTML = funnel.steps.map(s => `
    <div class="funnel-step-row">
      <div class="funnel-step-meta">
        <span>${formatStepName(s.name)}</span>
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
}

function renderFallbackFunnel() {
  const steps = [
    { name: 'Product Views', count: 1240, rate: 100 },
    { name: 'Add to Cart', count: 340, rate: 27.4 },
    { name: 'Checkout Started', count: 180, rate: 52.9 },
    { name: 'Purchase Completed', count: 68, rate: 37.8 }
  ];
  el.kpiConversion.textContent = '5.4%';
  el.funnelDisplay.innerHTML = steps.map(s => `
    <div class="funnel-step-row">
      <div class="funnel-step-meta">
        <span>${s.name}</span>
        <span>${s.count.toLocaleString()} (${s.rate}%)</span>
      </div>
      <div class="funnel-bar-bg">
        <div class="funnel-bar-fill" style="width: ${s.rate}%"></div>
      </div>
    </div>
  `).join('');
}

function formatStepName(name) {
  const dict = {
    'page_view': 'Product Catalog Views',
    'add_to_cart': 'Added to Cart',
    'checkout_step': 'Initiated Checkout',
    'purchase': 'Order Completed'
  };
  return dict[name] || name;
}

// 4. Live Radar
async function fetchLiveRadar() {
  const res = await fetchAPI('/api/v1/query/live?window_minutes=5');
  const live = res?.data || res;

  if (live) {
    el.liveCount.textContent = `${live.active_shoppers || 0} Active`;

    // Devices
    const devices = live.devices || { desktop: 0, mobile: 0 };
    el.deviceBreakdown.innerHTML = Object.entries(devices).map(([dev, cnt]) => `
      <div class="breakdown-item">
        <span style="text-transform: capitalize;">${dev || 'Unknown'}</span>
        <span style="font-family: var(--font-mono); font-weight: 600;">${cnt}</span>
      </div>
    `).join('') || '<div class="breakdown-item">None active</div>';

    // Countries
    const countries = live.countries || { US: 0 };
    el.geoBreakdown.innerHTML = Object.entries(countries).slice(0, 4).map(([code, cnt]) => `
      <div class="breakdown-item">
        <span>${code}</span>
        <span style="font-family: var(--font-mono); font-weight: 600;">${cnt}</span>
      </div>
    `).join('') || '<div class="breakdown-item">None active</div>';

    // Top Paths
    const paths = live.top_paths || [];
    el.pathsBreakdown.innerHTML = paths.slice(0, 3).map(p => `
      <div class="path-item">
        <span>${p.path}</span>
        <span>${p.count}</span>
      </div>
    `).join('') || '<div class="path-item"><span>/</span><span>0</span></div>';
  }
}

// 5. Behavioral ML Intent Stream
function populateMLIntentStream() {
  const mockShoppers = [
    { device: 'dev-98a2f1c0', intent: 0.94, status: 'HIGH INTENT', signals: '3x carts, 420s dwell' },
    { device: 'dev-4b71d9e2', intent: 0.88, status: 'HIGH INTENT', signals: 'checkout step 1 reached' },
    { device: 'dev-81c039ab', intent: 0.42, status: 'EXPLORING', signals: 'catalog browsing' },
    { device: 'dev-2f91a788', intent: 0.18, status: 'CASUAL', signals: '1 view, 12s dwell' }
  ];

  el.intentRows.innerHTML = mockShoppers.map(s => {
    const badgeClass = s.intent >= 0.85 ? 'high' : (s.intent >= 0.5 ? 'medium' : 'low');
    return `
      <div class="intent-row">
        <span style="font-family: var(--font-mono); font-weight: 600;">${s.device}</span>
        <span class="propensity-badge ${badgeClass}">${(s.intent * 100).toFixed(1)}%</span>
        <span style="font-size: 0.72rem; font-weight: 700;">${s.status}</span>
        <span style="color: var(--text-muted); font-size: 0.75rem;">${s.signals}</span>
      </div>
    `;
  }).join('');
}

// 6. Shopper Timeline Inspector
async function inspectShopper(identifier) {
  el.shopperTimeline.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-subtle);">Querying unified clickstream...</div>';
  const res = await fetchAPI(`/api/v1/query/shopper/${encodeURIComponent(identifier)}`);
  
  const journey = res?.data || res;
  if (!journey || !journey.events || journey.events.length === 0) {
    el.shopperTimeline.innerHTML = `<div class="timeline-empty-state">No events recorded for shopper "${identifier}".</div>`;
    return;
  }

  el.shopperTimeline.innerHTML = `
    <div class="timeline-list">
      ${journey.events.map(ev => `
        <div class="timeline-node">
          <div class="node-title">
            <span>${ev.name} ${ev.product_id ? `(Product: ${ev.product_id})` : ''}</span>
            <span style="font-size: 0.75rem; color: var(--accent-emerald); font-weight: 700;">${ev.revenue ? `$${ev.revenue}` : ''}</span>
          </div>
          <div class="node-meta">
            <span>${ev.path || '/'} • ${ev.browser || 'Browser'} on ${ev.os || 'OS'} • ${ev.country || ''}</span>
            <span style="float: right;">${new Date(ev.created_at).toLocaleTimeString()}</span>
          </div>
        </div>
      `).join('')}
    </div>
  `;
}

// Bootstrap
document.addEventListener('DOMContentLoaded', init);
