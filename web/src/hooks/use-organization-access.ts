export function useOrganizationAccess(_organizationId?: string | null) {
  return {
    role: 'org:admin',
    isAdmin: true,
  };
}
