import { Link, useNavigate } from '@tanstack/react-router';
import { AnimatePresence, motion } from 'framer-motion';
import {
  ChevronDownIcon,
  CogIcon,
  CreditCardIcon,
  LayoutListIcon,
  PlusIcon,
  UsersIcon,
  WorkflowIcon,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import { Badge } from './ui/badge';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from './ui/dropdown-menu';
import { useAppParams } from '@/hooks/use-app-params';
import { useAppContext } from '@/hooks/use-app-context';
import { useOrganizationAccess } from '@/hooks/use-organization-access';
import { pushModal } from '@/modals';
import type { RouterOutputs } from '@/trpc/client';
import { cn } from '@/utils/cn';

export default function SidebarOrganizationMenu({
  organization: _organization,
}: {
  organization: RouterOutputs['organization']['list'][number];
}) {
  const { organizationId, tenantId } = useAppParams();
  const activeTenantId = tenantId || organizationId || '';
  const { isAdmin } = useOrganizationAccess(activeTenantId);

  return (
    <>
      <Link
        activeOptions={{ exact: true }}
        className={cn(
          'flex items-center gap-2 rounded-md px-3 py-2 font-medium text-[13px] transition-all hover:bg-def-200'
        )}
        to={'/$tenantId' as any}
        params={{ tenantId: activeTenantId } as any}
      >
        <LayoutListIcon size={20} />
        <div className="flex-1">Projects</div>
      </Link>
      <Link
        activeOptions={{ exact: true }}
        className={cn(
          'flex items-center gap-2 rounded-md px-3 py-2 font-medium text-[13px] transition-all hover:bg-def-200'
        )}
        to={'/$tenantId/settings' as any}
        params={{ tenantId: activeTenantId } as any}
      >
        <CogIcon size={20} />
        <div className="flex-1">Settings</div>
      </Link>
      {isAdmin && (
        <Link
          className={cn(
            'flex items-center gap-2 rounded-md px-3 py-2 font-medium text-[13px] transition-all hover:bg-def-200'
          )}
          to={'/$tenantId/members' as any}
          params={{ tenantId: activeTenantId } as any}
        >
          <UsersIcon size={20} />
          <div className="flex-1">Members</div>
        </Link>
      )}
      <Link
        className={cn(
          'flex items-center gap-2 rounded-md px-3 py-2 font-medium text-[13px] transition-all hover:bg-def-200'
        )}
        to={'/$tenantId/integrations' as any}
        params={{ tenantId: activeTenantId } as any}
      >
        <WorkflowIcon size={20} />
        <div className="flex-1">Integrations</div>
      </Link>
    </>
  );
}

export function ActionCTAButton() {
  const navigate = useNavigate();
  const { organizationId, tenantId } = useAppParams();
  const activeTenantId = tenantId || organizationId || '';
  const { isAdmin } = useOrganizationAccess(activeTenantId);

  const ACTIONS = [
    ...(isAdmin
      ? [
          {
            label: 'Create a project',
            icon: PlusIcon,
            onClick: () => pushModal('AddProject'),
          },
          {
            label: 'Invite a user',
            icon: UsersIcon,
            onClick: () => pushModal('CreateInvite'),
          },
        ]
      : []),
    {
      label: 'Add integration',
      icon: WorkflowIcon,
      onClick: () =>
        navigate({
          to: '/$tenantId/integrations' as any,
          params: { tenantId: activeTenantId } as any,
        }),
    },
  ];

  const [currentActionIndex, setCurrentActionIndex] = useState(0);

  useEffect(() => {
    const interval = setInterval(() => {
      setCurrentActionIndex((prevIndex) => (prevIndex + 1) % ACTIONS.length);
    }, 2000);

    return () => clearInterval(interval);
  }, []);

  return (
    <div className="mb-4">
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
