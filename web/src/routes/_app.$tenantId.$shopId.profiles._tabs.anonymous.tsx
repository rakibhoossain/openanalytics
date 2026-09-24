import { ProfilesTable } from '@/components/profiles/table';
import { useDataTablePagination } from '@/components/ui/data-table/data-table-hooks';
import { useSearchQueryState } from '@/hooks/use-search-query-state';
import { useTableFilters } from '@/hooks/use-table-filters';
import { useTRPC } from '@/integrations/trpc/react';
import { PAGE_TITLES, createEntityTitle } from '@/utils/title';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute(
  '/_app/$tenantId/$shopId/profiles/_tabs/anonymous',
)({
  component: Component,
  head: () => {
    return {
      meta: [
        {
          title: createEntityTitle('Anonymous', PAGE_TITLES.PROFILES),
        },
      ],
    };
  },
});

function Component() {
  const params = Route.useParams() as any;
  const projectId = params.shopId || (params.shopId || params.projectId);
  const trpc = useTRPC();
  const { page } = useDataTablePagination(50);
  const { debouncedSearch } = useSearchQueryState();
  const [filters] = useTableFilters('f');
  const query = useQuery(
    trpc.profile.list.queryOptions(
      {
        cursor: page - 1,
        projectId,
        take: 50,
        search: debouncedSearch,
        isExternal: false,
        filters,
      },
      {
        placeholderData: keepPreviousData,
      },
    ),
  );

  return <ProfilesTable query={query} type="profiles" />;
}
