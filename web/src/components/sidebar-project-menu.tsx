import type { IServiceDashboards } from '@openpanel/db';
import { useNavigate } from '@tanstack/react-router';
import { AnimatePresence, motion } from 'framer-motion';
import {
  BellIcon,
  BookOpenIcon,
  Building2Icon,
  ChartLineIcon,
  ChevronDownIcon,
  CogIcon,
  GanttChartIcon,
  Globe2Icon,
  GridIcon,
  LayersIcon,
  LayoutDashboardIcon,
  LayoutPanelTopIcon,
  PlugIcon,
  PlusIcon,
  SearchIcon,
  SparklesIcon,
  TargetIcon,
  TrendingUpDownIcon,
  UndoDotIcon,
  UserCircleIcon,
  UsersIcon,
  WallpaperIcon,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import { SidebarLink } from './sidebar-link';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from './ui/dropdown-menu';
import { useChatState } from '@/components/chat/chat-context';
import { SidebarChatComposer } from '@/components/chat/sidebar-chat-composer';
import { useAppParams } from '@/hooks/use-app-params';
import { pushModal } from '@/modals';
import { cn } from '@/utils/cn';

interface SidebarProjectMenuProps {
  dashboards: IServiceDashboards;
}

export default function SidebarProjectMenu({
  dashboards,
}: SidebarProjectMenuProps) {
  return (
    <>
      <SidebarChatComposer />
      <div className="mb-2 font-medium text-muted-foreground text-sm">
        Analytics
      </div>
      <SidebarLink href={'/'} icon={WallpaperIcon} label="Overview" />
      <SidebarLink
        href={'/dashboards'}
        icon={LayoutPanelTopIcon}
        label="Dashboards"
      />
      <SidebarLink
        href={'/insights'}
        icon={TrendingUpDownIcon}
        label="Insights"
      />
      <SidebarLink
        href={'/ml-intent'}
        icon={SparklesIcon}
        label="Behavioral ML"
      />
      <SidebarLink href={'/pages'} icon={LayersIcon} label="Pages" />
      <SidebarLink href={'/realtime'} icon={Globe2Icon} label="Realtime" />
      <SidebarLink href={'/events'} icon={GanttChartIcon} label="Events" />
      <SidebarLink href={'/sessions'} icon={UsersIcon} label="Sessions" />
      <SidebarLink href={'/profiles'} icon={UserCircleIcon} label="Profiles" />
      <SidebarLink href={'/integrations'} icon={PlugIcon} label="Integrations" />
      <SidebarLink href={'..'} icon={UndoDotIcon} label="Back to workspace" />
    </>
  );
}

export function ActionCTAButton() {
  const navigate = useNavigate();
  const { openChatForContext } = useChatState();
  const { tenantId, shopId, organizationId, projectId } = useAppParams();
  const activeTenantId = tenantId || organizationId || '';
  const activeShopId = shopId || projectId || '';

  const ACTIONS = [
    {
      label: 'Create report',
      icon: ChartLineIcon,
      onClick: () =>
        navigate({
          to: '/$tenantId/$shopId/reports' as any,
          params: { tenantId: activeTenantId, shopId: activeShopId } as any,
        }),
    },
    {
      label: 'Ask AI',
      icon: SparklesIcon,
      onClick: () => openChatForContext(),
    },
    {
      label: 'Create dashboard',
      icon: LayoutDashboardIcon,
      onClick: () => pushModal('AddDashboard'),
    },
  ];

  const [currentActionIndex, setCurrentActionIndex] = useState(0);

  useEffect(() => {
    const interval = setInterval(() => {
      setCurrentActionIndex((prevIndex) => {
        const nextIndex = (prevIndex + 1) % ACTIONS.length;
        if (nextIndex === 0 && prevIndex !== 0) {
          clearInterval(interval);
          return 0;
        }
        return nextIndex;
      });
    }, 2000);

    return () => clearInterval(interval);
  }, []);

  return (
    <div className="mb-2">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            className={cn(
              'group flex w-full items-center gap-2 rounded-md border border-border bg-def-200 px-3 py-2 text-left',
              'text-[13px] font-medium text-foreground',
              'transition-colors hover:bg-def-300',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
            )}
          >
            <PlusIcon className="size-5 shrink-0" />
            <div className="relative flex h-5 flex-1 items-center overflow-hidden">
              <AnimatePresence mode="popLayout">
                <motion.span
                  animate={{ y: 0, opacity: 1 }}
                  className="absolute whitespace-nowrap"
                  exit={{ y: -16, opacity: 0 }}
                  initial={{ y: 16, opacity: 0 }}
                  key={currentActionIndex}
                  transition={{
                    type: 'spring',
                    stiffness: 300,
                    damping: 25,
                    duration: 0.3,
                  }}
                >
                  {ACTIONS[currentActionIndex].label}
                </motion.span>
              </AnimatePresence>
            </div>
            <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-56">
          {ACTIONS.map((action) => (
            <DropdownMenuItem
              className="cursor-pointer"
              key={action.label}
              onClick={action.onClick}
            >
              <action.icon className="mr-2 size-4" />
              {action.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
