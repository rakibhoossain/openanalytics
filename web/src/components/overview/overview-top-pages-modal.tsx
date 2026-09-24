import { useEventQueryFilters } from '@/hooks/use-event-query-filters';

import { useTRPC } from '@/integrations/trpc/react';
import { useQuery } from '@tanstack/react-query';
import { ExternalLinkIcon } from 'lucide-react';
import { SerieIcon } from '../report-chart/common/serie-icon';
import { Tooltiper } from '../ui/tooltip';
import { OverviewListModal } from './overview-list-modal';
import { useOverviewOptions } from './useOverviewOptions';

interface OverviewTopPagesProps {
  projectId: string;
}

export default function OverviewTopPagesModal({
  projectId,
}: OverviewTopPagesProps) {
  const [filters, setFilter] = useEventQueryFilters();
  const { startDate, endDate, range } = useOverviewOptions();
  const trpc = useTRPC();
  const query = useQuery(
    trpc.overview.topPages.queryOptions({
      projectId,
      filters,
      startDate,
      endDate,
      mode: 'page',
      range,
    }),
  );

  return (
    <OverviewListModal
      title="Top Pages"
      searchPlaceholder="Search pages..."
      data={query.data ?? []}
      keyExtractor={(item) => (item.path || '') + (item.origin || '')}
      searchFilter={(item, query) =>
        ((item.path || '') + (item.origin || '')).toLowerCase().includes(query.toLowerCase())
      }
      columnName="Path"
      renderItem={(item) => {
        const path = item.path || '';
        const origin = item.origin || '';
        const fullUrl = origin + path;
        return (
          <Tooltiper asChild content={fullUrl || 'Not set'} side="left">
            <div className="flex items-center gap-2 min-w-0">
              <SerieIcon name={origin} />
              <button
                type="button"
                className="truncate hover:underline"
                onClick={() => {
                  if (path) setFilter('path', path);
                  if (origin) setFilter('origin', origin);
                }}
              >
                {path || <span className="opacity-40">Not set</span>}
              </button>
              <a
                href={fullUrl || '#'}
                target="_blank"
                rel="noreferrer"
                onClick={(e) => e.stopPropagation()}
                className="flex-shrink-0"
              >
                <ExternalLinkIcon className="size-3 opacity-0 group-hover/row:opacity-100 transition-opacity" />
              </a>
            </div>
          </Tooltiper>
        );
      }}
    />
  );
}
