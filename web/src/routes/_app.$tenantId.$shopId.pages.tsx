import { PagesTable } from '@/components/pages/table';
import { PageContainer } from '@/components/page-container';
import { PageHeader } from '@/components/page-header';
import { useRangePageContext } from '@/hooks/use-page-context-helpers';
import { PAGE_TITLES, createProjectTitle } from '@/utils/title';
import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_app/$tenantId/$shopId/pages')({
  component: Component,
  head: () => ({
    meta: [{ title: createProjectTitle(PAGE_TITLES.PAGES) }],
  }),
});

function Component() {
  const params = Route.useParams() as any;
  const projectId = params.shopId || (params.shopId || params.projectId);
  useRangePageContext('pages');
  return (
    <PageContainer>
      <PageHeader title="Pages" description="Access all your pages here" className="mb-8" />
      <PagesTable projectId={projectId} />
    </PageContainer>
  );
}
