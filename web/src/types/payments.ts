export type IPolarProduct = {
  id: string;
  name: string;
  description?: string | null;
  prices: Array<{
    id: string;
    priceAmount?: number;
    price_amount?: number;
    recurringInterval?: string;
  }>;
  metadata?: Record<string, any>;
};

export type SubscriptionStateMeta = {
  badge: { label: string; variant: 'default' | 'secondary' | 'success' | 'warning' | 'destructive' } | null;
  banner: {
    title: string;
    description: string;
    cta: string;
    tone: 'default' | 'warning' | 'destructive';
  } | null;
  statusLine: { text: string; tone: 'default' | 'warning' | 'destructive' } | null;
  blockType: 'expired' | 'trialEnded' | 'unpaid' | null;
};

export function getSubscriptionStateMeta(
  _state: string,
  _opts?: { endsAt?: Date | null; canceledAt?: Date | null }
): SubscriptionStateMeta {
  return {
    badge: null,
    banner: null,
    statusLine: null,
    blockType: null,
  };
}
