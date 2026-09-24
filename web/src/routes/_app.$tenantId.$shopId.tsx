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
    await Promise.all([
      context.queryClient.prefetchQuery(
        context.trpc.organization.get.queryOptions({
          organizationId: (params.tenantId || params.organizationId),
        }),
      ),
      context.queryClient.prefetchQuery(
        context.trpc.project.getProjectWithClients.queryOptions({
          projectId: (params.shopId || params.projectId),
        }),
      ),
    ]);
  },
});

function ProjectDashboard() {
  const params = Route.useParams() as any;
  const organizationId = params.tenantId || (params.tenantId || params.organizationId);
  const projectId = params.shopId || (params.shopId || params.projectId);
  const trpc = useTRPC();
  useSuspenseQuery(
    trpc.organization.get.queryOptions({
      organizationId,
    }),
  );
  const { data: project } = useSuspenseQuery(
    trpc.project.getProjectWithClients.queryOptions({ projectId }),
  );
  useProjectDocumentTitle(project?.name);

  return <Outlet />;
}
