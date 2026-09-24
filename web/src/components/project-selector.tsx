import type { IServiceOrganization } from '@openpanel/db';
import { Link, useRouter } from '@tanstack/react-router';
import {
  Building2Icon,
  CheckIcon,
  ChevronsUpDownIcon,
  PlusIcon,
} from 'lucide-react';
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useAppParams } from '@/hooks/use-app-params';
import { useOrganizationAccess } from '@/hooks/use-organization-access';
import { pushModal } from '@/modals';

interface ProjectSelectorProps {
  projects: Array<{ id: string; name: string; organizationId: string }>;
  organizations?: IServiceOrganization[];
  align?: 'start' | 'end';
}

export default function ProjectSelector({
  projects,
  organizations,
  align = 'start',
}: ProjectSelectorProps) {
  const router = useRouter();
  const { organizationId, projectId, tenantId, shopId } = useAppParams();
  const activeTenantId = tenantId || organizationId || '';
  const activeShopId = shopId || projectId || '';
  const { isAdmin } = useOrganizationAccess(activeTenantId);
  const [open, setOpen] = useState(false);

  const changeProject = (newProjectId: string) => {
    router.navigate({
      to: '/$tenantId/$shopId' as any,
      params: {
        tenantId: activeTenantId,
        shopId: newProjectId,
      } as any,
    });
  };

  const changeOrganization = (newOrganizationId: string) => {
    router.navigate({
      to: '/$tenantId' as any,
      params: {
        tenantId: newOrganizationId,
      } as any,
    });
  };

  return (
    <DropdownMenu onOpenChange={setOpen} open={open}>
      <DropdownMenuTrigger asChild>
        <Button
          aria-expanded={open}
          className="flex min-w-0 flex-1 items-center justify-start"
          role="combobox"
          size={'sm'}
          variant="outline"
        >
          <Building2Icon className="shrink-0" size={16} />
          <span className="mx-2 truncate">
            {activeShopId
              ? (projects.find((p) => p.id === activeShopId)?.name || 'Primary Store')
              : activeTenantId
                ? (organizations?.find((o) => o.id === activeTenantId)?.name || 'Default Organization')
                : 'Select project'}
          </span>
          <ChevronsUpDownIcon className="ml-auto h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={align} className="w-[200px]">
        <DropdownMenuLabel>Projects</DropdownMenuLabel>
        <DropdownMenuGroup>
          {projects.slice(0, 10).map((project) => (
            <DropdownMenuItem
              key={project.id}
              onClick={() => changeProject(project.id)}
            >
              {project.name}
              {project.id === activeShopId && (
                <DropdownMenuShortcut>
                  <CheckIcon size={16} />
                </DropdownMenuShortcut>
              )}
            </DropdownMenuItem>
          ))}
          {projects.length > 10 && (
            <DropdownMenuItem asChild>
              <Link
                params={{
                  tenantId: activeTenantId,
                } as any}
                to={'/$tenantId' as any}
              >
                All projects
              </Link>
            </DropdownMenuItem>
          )}
          {isAdmin && (
            <DropdownMenuItem
              className="text-emerald-600"
              onClick={() => {
                pushModal('AddProject');
              }}
            >
              Create new project
              <DropdownMenuShortcut>
                <PlusIcon size={16} />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          )}
        </DropdownMenuGroup>
        {!!organizations && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuLabel>Organizations</DropdownMenuLabel>
            <DropdownMenuGroup>
              {organizations.map((organization) => (
                <DropdownMenuItem
                  key={organization.id}
                  onClick={() => changeOrganization(organization.id)}
                >
                  {organization.name}
                  {organization.id === activeTenantId && (
                    <DropdownMenuShortcut>
                      <CheckIcon size={16} />
                    </DropdownMenuShortcut>
                  )}
                </DropdownMenuItem>
              ))}
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild>
                <Link to={'/onboarding/project'}>
                  New organization
                  <DropdownMenuShortcut>
                    <PlusIcon size={16} />
                  </DropdownMenuShortcut>
                </Link>
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
