import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute(
  '/_app/$tenantId/$shopId/events/_tabs/',
)({
  component: Component,
  beforeLoad({ params }) {
    throw redirect({
      to: '/$tenantId/$shopId/events/events',
      params,
    });
  },
});

function Component() {
  return null;
}
