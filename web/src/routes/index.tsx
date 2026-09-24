import { createFileRoute, redirect } from '@tanstack/react-router';
import FullPageLoadingState from '@/components/full-page-loading-state';

export const Route = createFileRoute('/')({
  beforeLoad: () => {
    throw redirect({
      to: '/$tenantId/$shopId',
      params: {
        tenantId: '018e69d0-7a89-7000-8b1a-200000000001',
        shopId: '018e69d0-7a89-7000-8b1a-200000000002',
      } as any,
    });
  },
  pendingComponent: FullPageLoadingState,
  component: () => null,
});
