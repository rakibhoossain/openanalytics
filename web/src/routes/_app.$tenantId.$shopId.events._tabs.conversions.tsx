import { EventsTable } from '@/components/events/table';
import { useReadColumnVisibility } from '@/components/ui/data-table/data-table-hooks';
import { useEventQueryNamesFilter } from '@/hooks/use-event-query-filters';
import { useTRPC } from '@/integrations/trpc/react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';
import { parseAsIsoDateTime, useQueryState } from 'nuqs';

export const Route = createFileRoute(
  '/_app/$tenantId/$shopId/events/_tabs/conversions',
)({
  component: Component,
});

function Component() {
  const params = Route.useParams() as any;
  const projectId = params.shopId || (params.shopId || params.projectId);
  const trpc = useTRPC();
  const [startDate, setStartDate] = useQueryState(
    'startDate',
    parseAsIsoDateTime,
  );
  const [endDate, setEndDate] = useQueryState('endDate', parseAsIsoDateTime);
  const [eventNames] = useEventQueryNamesFilter();
  const columnVisibility = useReadColumnVisibility('events');
  const query = useInfiniteQuery(
    trpc.event.conversions.infiniteQueryOptions(
      {
        projectId,
        startDate: startDate || undefined,
        endDate: endDate || undefined,
        events: eventNames,
        columnVisibility: columnVisibility ?? {},
      },
      {
        getNextPageParam: (lastPage: any) => lastPage?.meta?.next ?? undefined,
      },
    ),
  );

  return <EventsTable query={query} />;
}
