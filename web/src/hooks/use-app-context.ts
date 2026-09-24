import { useRouteContext } from '@tanstack/react-router';

export function useAppContext() {
  const params = useRouteContext({
    strict: false,
  });

  return {
    apiUrl: params?.apiUrl || 'http://localhost:8081',
    dashboardUrl: params?.dashboardUrl || 'http://localhost:3000',
    isSelfHosted: params?.isSelfHosted ?? true,
    isMaintenance: params?.isMaintenance ?? false,
    isDemo: params?.isDemo ?? false,
  };
}
