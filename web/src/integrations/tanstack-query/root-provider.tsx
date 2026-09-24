import type { AppRouter } from '@openpanel/trpc';
import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query';
import { createIsomorphicFn } from '@tanstack/react-start';
import { getRequestHeaders } from '@tanstack/react-start/server';
import { createTRPCClient, httpLink, TRPCClientError } from '@trpc/client';
import { createTRPCOptionsProxy } from '@trpc/tanstack-react-query';
import { useMemo } from 'react';
import superjson from 'superjson';
import { TRPCProvider } from '@/integrations/trpc/react';

const DEFAULT_RETRY_COUNT = 1;

function shouldRetryQuery(failureCount: number, error: unknown) {
  if (error instanceof TRPCClientError) {
    const status = error.data?.httpStatus;
    if (typeof status === 'number' && status >= 400 && status < 500) {
      return false;
    }
  }
  return failureCount < DEFAULT_RETRY_COUNT;
}

function handleUnauthorized(_error: unknown) {
  // Self-hosted: no login redirect
}

// Resolve the tRPC base URL per environment. During SSR the server can reach
// the API over an internal address (e.g. a Docker/K8s service name) that the
// browser can't resolve, so `API_URL_SSR` overrides the public `apiUrl`
// server-side only. The client always uses the public `apiUrl` it was given.
const getSsrApiUrlOverride = createIsomorphicFn()
  .server(() => {
    return process.env.API_URL_SSR || process.env.API_URL || 'http://localhost:8081';
  })
  .client(() => undefined);

export const getIsomorphicHeaders = createIsomorphicFn()
  .server(() => {
    const headers = getRequestHeaders();
    const result: Record<string, string> = {};
    const cookie = headers.get('Cookie');
    if (cookie) {
      result.cookie = cookie;
    }
    return result;
  })
  .client(() => {
    return {};
  });

// Create a function that returns a tRPC client with optional cookies
export function createTRPCClientWithHeaders(apiUrl: string) {
  const ssrOverride = getSsrApiUrlOverride();
  const cleanApiUrl = apiUrl && apiUrl !== 'undefined' ? apiUrl : '';
  const baseUrl = ssrOverride || cleanApiUrl;
  return createTRPCClient<AppRouter>({
    links: [
      httpLink({
        transformer: superjson,
        url: `${baseUrl}/trpc`,
        headers: () => getIsomorphicHeaders(),
        fetch: async (url, options) => {
          try {
            const response = await fetch(url, {
              ...options,
              mode: 'cors',
              credentials: 'include',
            });

            if (typeof window === 'undefined') {
              const text = await response.clone().text();
              console.log('[tRPC fetch SSR]', url.toString(), response.status, text.slice(0, 100));
            }

            return response;
          } catch (error) {
            // Log fetch errors on server
            if (typeof window === 'undefined') {
              console.error('[tRPC SSR Error]', {
                url: url.toString(),
                error: error instanceof Error ? error.message : String(error),
                stack: error instanceof Error ? error.stack : undefined,
                options,
              });
            }
            throw error;
          }
        },
      }),
    ],
  });
}

export function getContext(apiUrl: string) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 1000 * 60 * 5,
        gcTime: 1000 * 60 * 10,
        refetchOnReconnect: false,
        retry: shouldRetryQuery,
      },
      dehydrate: { serializeData: superjson.serialize },
      hydrate: { deserializeData: superjson.deserialize },
    },
    queryCache: new QueryCache({ onError: handleUnauthorized }),
    mutationCache: new MutationCache({ onError: handleUnauthorized }),
  });

  // Create a tRPC client with cookies if provided
  const client = createTRPCClientWithHeaders(apiUrl);

  const serverHelpers = createTRPCOptionsProxy({
    client,
    queryClient,
  });
  return {
    queryClient,
    trpc: serverHelpers,
  };
}

export function Provider({
  children,
  queryClient,
  apiUrl,
}: {
  children: React.ReactNode;
  queryClient: QueryClient;
  apiUrl: string;
}) {
  const trpcClient = useMemo(
    () => createTRPCClientWithHeaders(apiUrl),
    [apiUrl]
  );
  return (
    <TRPCProvider queryClient={queryClient} trpcClient={trpcClient}>
      {children}
    </TRPCProvider>
  );
}
