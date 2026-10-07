import debounce from 'lodash.debounce';
import { useCallback, useMemo } from 'react';
import { useWebSocket } from 'react-use-websocket/dist/lib/use-websocket';

import { getSuperJson } from '@openpanel/json';
import { useAppContext } from './use-app-context';
import { useAppParams } from './use-app-params';

type UseWSOptions = {
  debounce?: {
    delay: number;
    maxWait?: number;
  };
};

function resolveWsUrl(path: string, apiUrl?: string, token?: string): string {
  const query = token ? `?token=${encodeURIComponent(token)}` : '';
  const cleanPath = path.split('?')[0];

  if (typeof window !== 'undefined') {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    if (apiUrl && !apiUrl.includes('localhost') && !apiUrl.includes('127.0.0.1')) {
      const wsProto = apiUrl.startsWith('https') ? 'wss:' : 'ws:';
      return `${apiUrl.replace(/^https?:/, wsProto)}${cleanPath}${query}`;
    }
    return `${proto}//${window.location.host}${cleanPath}${query}`;
  }
  const ws = (apiUrl || 'http://localhost:8081').replace(/^https/, 'wss').replace(/^http/, 'ws');
  return `${ws}${cleanPath}${query}`;
}

async function fetchWsToken(apiUrl: string | undefined, shopId: string, tenantId: string): Promise<string | null> {
  if (!shopId) return null;
  try {
    const cleanApiUrl = apiUrl && apiUrl !== 'undefined' ? apiUrl : '';
    const res = await fetch(`${cleanApiUrl}/api/v1/auth/ws-token`, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json',
        'X-Shop-ID': shopId,
        'X-Tenant-ID': tenantId || '',
      },
    });
    if (!res.ok) return null;
    const body = await res.json();
    return body?.data?.token || body?.token || null;
  } catch {
    return null;
  }
}

export default function useWS<T>(
  path: string,
  onMessage: (event: T) => void,
  options?: UseWSOptions,
) {
  const context = useAppContext();
  const params = useAppParams();

  // Extract target shopId from path (e.g. /live/visitors/:projectId) or params or localStorage
  const pathParts = path.split('?')[0].split('/').filter(Boolean);
  const lastPart = pathParts.length > 0 ? pathParts[pathParts.length - 1] : '';
  const isNamedRoute = ['visitors', 'events', 'live', 'notifications', 'organization'].includes(lastPart.toLowerCase());
  const shopIdFromPath = !isNamedRoute ? lastPart : '';

  const shopId =
    shopIdFromPath ||
    params.shopId ||
    params.projectId ||
    (typeof window !== 'undefined' ? localStorage.getItem('active_shop_id') || '' : '');

  const tenantId =
    params.tenantId ||
    params.organizationId ||
    (typeof window !== 'undefined' ? localStorage.getItem('active_tenant_id') || '' : '') ||
    shopId;

  const debouncedOnMessage = useMemo(() => {
    if (options?.debounce) {
      return debounce(onMessage, options.debounce.delay, options.debounce);
    }
    return onMessage;
  }, [options?.debounce?.delay, onMessage]);

  const getSocketUrl = useCallback(async (): Promise<string> => {
    if (!shopId) return '';
    const token = await fetchWsToken(context.apiUrl, shopId, tenantId);
    return resolveWsUrl(path, context.apiUrl, token || undefined);
  }, [context.apiUrl, path, shopId, tenantId]);

  useWebSocket(
    shopId ? getSocketUrl : null,
    {
      shouldReconnect: () => true,
      reconnectInterval: 2000,
      reconnectAttempts: 100,
      retryOnError: true,
      onMessage(event) {
        try {
          const data = getSuperJson<T>(event.data);
          if (data !== null && data !== undefined) {
            debouncedOnMessage(data);
          }
        } catch (error) {
          console.error('[WS] Error parsing message', error);
        }
      },
    },
    Boolean(shopId),
  );
}
