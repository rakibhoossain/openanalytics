# OpenAnalytics Executive Dashboard UI

High-performance real-time telemetry dashboard for e-commerce merchants and executives.

## Architecture

- **Backend Query Engine**: Connects to `cmd/query` on `http://localhost:8081` (Native ClickHouse TCP on port 9000).
- **Design System**: Vanilla CSS, Dark theme, glassmorphism, responsive grid layout, and neon micro-animations.
- **Multi-Tenant Scoping**: All requests pass `X-Tenant-ID` and `X-Shop-ID` headers containing UUIDv7 identifiers.
- **Real-Time Live Radar**: Polls active shoppers, device breakdown, and geographic distribution.
- **Behavioral ML Intent Stream**: Visualizes real-time conversion propensity and cart abandonment probabilities.

## Integration with `ai-cart-dashboard`

You can embed OpenAnalytics directly into `ai-cart-dashboard` via an iframe or mount the static files directly:

```html
<iframe 
  src="http://localhost:8081/ui" 
  width="100%" 
  height="900px" 
  frameborder="0"
  style="border-radius: 12px; border: 1px solid rgba(255,255,255,0.1);"
></iframe>
```

Or pass tenant authentication via window message bridge:
```javascript
window.postMessage({
  type: 'OPENANALYTICS_SET_TENANT',
  tenantId: merchantJwt.tenantId,
  shopId: merchantJwt.shopId
}, '*');
```

## Running Locally

To preview the dashboard:
```bash
# Serve static files on port 3005
npx serve ui -p 3005
# Or open ui/index.html in any modern browser
```
