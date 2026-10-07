import { useAppParams } from '@/hooks/use-app-params';
import useWS from '@/hooks/use-ws';
import type { Notification } from '@openpanel/db';
import { BellIcon } from 'lucide-react';
import { toast } from 'sonner';

export function NotificationProvider() {
  const { projectId } = useAppParams();

  if (!projectId) return null;

  return <InnerNotificationProvider projectId={projectId} />;
}

export function InnerNotificationProvider({
  _projectId,
}: { _projectId?: string; projectId?: string }) {
  // Live notifications route was deprecated and removed from the Go engine.
  return null;
}
