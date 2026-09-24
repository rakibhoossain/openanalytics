import debounce from 'lodash.debounce';
import { useEffect, useMemo, useState } from 'react';
import { useWebSocket } from 'react-use-websocket/dist/lib/use-websocket';

import { getSuperJson } from '@openpanel/json';
import { useAppContext } from './use-app-context';

type UseWSOptions = {
  debounce?: {
    delay: number;
    maxWait?: number;
  };
};

function resolveWsUrl(path: string, apiUrl?: string): string {
  if (typeof window !== 'undefined') {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    if (apiUrl && !apiUrl.includes('localhost') && !apiUrl.includes('127.0.0.1')) {
      const wsProto = apiUrl.startsWith('https') ? 'wss:' : 'ws:';
      return `${apiUrl.replace(/^https?:/, wsProto)}${path}`;
    }
    return `${proto}//${window.location.host}${path}`;
  }
  const ws = (apiUrl || 'http://localhost:8081').replace(/^https/, 'wss').replace(/^http/, 'ws');
  return `${ws}${path}`;
}

export default function useWS<T>(
  path: string,
  onMessage: (event: T) => void,
  options?: UseWSOptions,
) {
  const context = useAppContext();
  const [baseUrl, setBaseUrl] = useState(() => resolveWsUrl(path, context.apiUrl));

  const debouncedOnMessage = useMemo(() => {
    if (options?.debounce) {
      return debounce(onMessage, options.debounce.delay, options.debounce);
    }
    return onMessage;
  }, [options?.debounce?.delay, onMessage]);

  useEffect(() => {
    const nextUrl = resolveWsUrl(path, context.apiUrl);
    if (baseUrl !== nextUrl) {
      setBaseUrl(nextUrl);
    }
  }, [path, context.apiUrl, baseUrl]);

  useWebSocket(baseUrl, {
    shouldReconnect: () => true,
    reconnectInterval: 2000,
    reconnectAttempts: 100,
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
  });
}
