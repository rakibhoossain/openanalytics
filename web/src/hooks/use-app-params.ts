import { useParams } from '@tanstack/react-router';

export function useAppParams() {
  const params = useParams({
    strict: false,
  }) as {
    organizationId?: string;
    projectId?: string;
    tenantId?: string;
    shopId?: string;
    [key: string]: any;
  };

  const tenantId = params?.tenantId || params?.organizationId || '';
  const shopId = params?.shopId || params?.projectId || '';

  return {
    ...params,
    tenantId,
    shopId,
    organizationId: tenantId,
    projectId: shopId,
  };
}
