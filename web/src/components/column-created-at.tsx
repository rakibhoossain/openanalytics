import { formatDateTime, timeAgo } from '@/utils/date';

export function ColumnCreatedAt({ children }: { children?: Date | string | null }) {
  if (!children) {
    return <div className="text-muted-foreground">-</div>;
  }
  const dateObj = typeof children === 'string' ? new Date(children) : children;
  if (!dateObj || isNaN(dateObj.getTime())) {
    return <div className="text-muted-foreground">-</div>;
  }

  return (
    <div className="relative">
      <div className="absolute inset-0 opacity-0 group-hover/row:opacity-100 transition-opacity duration-100">
        {formatDateTime(dateObj)}
      </div>
      <div className="text-muted-foreground group-hover/row:opacity-0 transition-opacity duration-100">
        {timeAgo(dateObj)}
      </div>
    </div>
  );
}

ColumnCreatedAt.size = 150;
