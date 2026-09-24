import { EventsTable } from '@/components/events/table';
import { useReadColumnVisibility } from '@/components/ui/data-table/data-table-hooks';
import {
  useEventQueryFilters,
  useEventQueryNamesFilter,
} from '@/hooks/use-event-query-filters';
import { useTRPC } from '@/integrations/trpc/react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';
import { parseAsIsoDateTime, useQueryState } from 'nuqs';

export const Route = createFileRoute(
  '/_app/$tenantId/$shopId/events/_tabs/events',
)({
  component: Component,
});

function Component() {
  const params = Route.useParams() as any;
  const projectId = params.shopId || (params.shopId || params.projectId);
  const trpc = useTRPC();
  const [filters] = useEventQueryFilters();
  const [startDate] = useQueryState('startDate', parseAsIsoDateTime);
  const [endDate] = useQueryState('endDate', parseAsIsoDateTime);
  const [eventNames] = useEventQueryNamesFilter();
  const columnVisibility = useReadColumnVisibility('events');

  const query = useInfiniteQuery(
    trpc.event.events.infiniteQueryOptions(
      {
        projectId,
        filters,
        events: eventNames,
        profileId: '',
        startDate: startDate || undefined,
        endDate: endDate || undefined,
        columnVisibility: columnVisibility ?? {},
      },
      {
        enabled: columnVisibility !== null,
        getNextPageParam: (lastPage: any) => lastPage?.meta?.next ?? undefined,
      },
    ),
  );

  return <EventsTable query={query} showEventListener />;
}
