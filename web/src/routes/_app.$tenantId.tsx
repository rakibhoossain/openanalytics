import { FullPageEmptyState } from '@/components/full-page-empty-state';
import FullPageLoadingState from '@/components/full-page-loading-state';
import { LinkButton } from '@/components/ui/button';
import { useTRPC } from '@/integrations/trpc/react';
import { useSuspenseQuery } from '@tanstack/react-query';
import {
  Outlet,
  createFileRoute,
  notFound,
} from '@tanstack/react-router';
import { Building2Icon } from 'lucide-react';

const IGNORE_ORGANIZATION_IDS = ['.well-known', 'onboarding', 'assets'];

const FILE_EXTENSIONS = [
  'jpg',
  'jpeg',
  'png',
  'gif',
  'bmp',
  'tiff',
  'ico',
  'js',
  'xml',
  'txt',
  'json',
  'webmanifest',
];

const isStaticFile = (path: string) => {
  return FILE_EXTENSIONS.some((extension) => path.endsWith(`.${extension}`));
};

export const Route = createFileRoute('/_app/$tenantId')({
  component: Component,
  beforeLoad: async ({ params }) => {
    if (IGNORE_ORGANIZATION_IDS.includes((params.tenantId || params.organizationId))) {
      throw notFound();
    }
    if (isStaticFile((params.tenantId || params.organizationId))) {
      throw notFound();
    }
  },
  loader: async ({ context, params }) => {
    await context.queryClient.prefetchQuery(
      context.trpc.organization.get.queryOptions({
        organizationId: (params.tenantId || params.organizationId),
      }),
    );
  },
  pendingComponent: FullPageLoadingState,
  notFoundComponent: () => (
    <FullPageEmptyState
      title="Workspace not found"
      description="This workspace doesn't exist or you don't have access to it."
      icon={Building2Icon}
      className="min-h-[calc(100vh-4rem)]"
    >
      <LinkButton href="/">Go to home</LinkButton>
    </FullPageEmptyState>
  ),
});

function Component() {
  const params = Route.useParams() as any;
  const organizationId = params.tenantId || (params.tenantId || params.organizationId);
  const trpc = useTRPC();
  useSuspenseQuery(
    trpc.organization.get.queryOptions({
      organizationId,
    }),
  );

  return <Outlet />;
}
