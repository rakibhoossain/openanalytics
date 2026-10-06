import { createFileRoute, redirect } from '@tanstack/react-router';
import FullPageLoadingState from '@/components/full-page-loading-state';

export const Route = createFileRoute('/')({
  beforeLoad: () => {
    throw redirect({
      to: '/$tenantId/$shopId',
      params: {
        tenantId: '019f5bfa-f6e4-76c0-9929-ed0daba7b14b',
        shopId: '019fc2f1-6be1-7a2d-9ebf-9f7dced8ccc1',
      } as any,
    });
  },
  pendingComponent: FullPageLoadingState,
  component: () => null,
});
