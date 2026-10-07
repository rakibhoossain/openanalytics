import { useEffect } from 'react';
import { useProjectDocumentTitle } from '@/hooks/use-project-document-title';
import { useTRPC } from '@/integrations/trpc/react';
import { PAGE_TITLES, createProjectTitle } from '@/utils/title';
import { useSuspenseQuery } from '@tanstack/react-query';
import { Outlet, createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_app/$tenantId/$shopId')({
  component: ProjectDashboard,
  head: () => {
    return {
      meta: [
        {
          title: createProjectTitle(PAGE_TITLES.DASHBOARD),
        },
      ],
    };
  },
  loader: async ({ context, params }) => {
    const p = params as any;
    await Promise.all([
      context.queryClient.prefetchQuery(
        context.trpc.organization.get.queryOptions({
          organizationId: (p.tenantId || p.organizationId),
        }),
      ),
      context.queryClient.prefetchQuery(
        context.trpc.project.getProjectWithClients.queryOptions({
          projectId: (p.shopId || p.projectId),
        }),
      ),
    ]);
  },
});

function ProjectDashboard() {
  const params = Route.useParams() as any;
  const organizationId = params.tenantId || (params.tenantId || params.organizationId);
  const projectId = params.shopId || (params.shopId || params.projectId);

  useEffect(() => {
    if (typeof window !== 'undefined') {
      if (organizationId) localStorage.setItem('active_tenant_id', organizationId);
      if (projectId) localStorage.setItem('active_shop_id', projectId);
    }
  }, [organizationId, projectId]);

  const trpc = useTRPC();
  useSuspenseQuery(
    trpc.organization.get.queryOptions({
      organizationId,
    }),
  );
  const { data: project } = useSuspenseQuery(
    trpc.project.getProjectWithClients.queryOptions({ projectId }),
  );
  useProjectDocumentTitle((project as any)?.name);

  return <Outlet />;
}

