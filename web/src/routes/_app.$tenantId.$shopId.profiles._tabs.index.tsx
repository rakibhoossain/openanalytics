import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute(
  '/_app/$tenantId/$shopId/profiles/_tabs/',
)({
  component: Component,
  beforeLoad({ params }) {
    throw redirect({
      to: '/$tenantId/$shopId/profiles/identified',
      params,
    });
  },
});

function Component() {
  return null;
}
