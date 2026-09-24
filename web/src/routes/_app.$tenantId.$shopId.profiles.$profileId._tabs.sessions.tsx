import { SessionsTable } from '@/components/sessions/table';
import { useSearchQueryState } from '@/hooks/use-search-query-state';
import { useTRPC } from '@/integrations/trpc/react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute(
  '/_app/$tenantId/$shopId/profiles/$profileId/_tabs/sessions',
)({
  component: Component,
});

function Component() {
  const params = Route.useParams() as any;
  const projectId = params.shopId || params.projectId;
  const profileId = params.profileId;
  const trpc = useTRPC();
  const { debouncedSearch } = useSearchQueryState();

  const query = useInfiniteQuery(
    trpc.session.list.infiniteQueryOptions(
      {
        projectId,
        profileId,
        take: 50,
        search: debouncedSearch,
      },
      {
        getNextPageParam: (lastPage: any) => lastPage?.meta?.next ?? undefined,
      },
    ),
  );

  return <SessionsTable query={query} />;
}
