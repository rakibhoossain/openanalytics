import { queryOptions } from '@tanstack/react-query';
import { createServerFn } from '@tanstack/react-start';

export const getServerEnvs = createServerFn().handler(() => {
  const envs = {
    apiUrl: process.env.API_URL || process.env.NEXT_PUBLIC_API_URL || '',
    dashboardUrl:
      process.env.DASHBOARD_URL ||
      process.env.NEXT_PUBLIC_DASHBOARD_URL ||
      'http://localhost:3000',
    isSelfHosted: true,
    isMaintenance: false,
    isDemo: false,
  };

  return envs;
});

export const getServerEnvsQueryOptions = queryOptions({
  queryKey: ['server-envs'],
  queryFn: getServerEnvs,
  staleTime: Number.POSITIVE_INFINITY,
});
