"""
High-Fidelity Synthetic E-Commerce Clickstream Generator
Generates realistic shopper sessions with diverse behavioral archetypes:
1. Quick Bounce (Window shoppers, 1-2 views, <30s dwell)
2. Casual Explorer (Broad catalog browsing, low carting)
3. Price Hunter (Sale items, comparison browsing, discount codes)
4. Cart Abandoner (High dwell, cart additions, drop off at checkout)
5. Decisive Buyer (Fast, focused views, cart additions, high purchase intent)
"""

import math
import random
from dataclasses import dataclass, field
from typing import List, Dict, Any, Tuple

@dataclass
class ShopperJourney:
    device_id: str
    archetype: str
    views_count: int
    cart_adds_count: int
    cart_removes_count: int
    checkout_starts_count: int
    distinct_products: int
    distinct_categories: int
    total_dwell_seconds: int
    avg_scroll_depth: float
    clicks_count: int
    sale_item_views: int
    cart_velocity_seconds: float
    device_type: str # "desktop" or "mobile"
    traffic_channel: str # "direct", "organic_search", "paid_ad", "social", "referral"
    # Target Labels
    has_purchase: int
    has_cart_abandoned: int
    has_churned: int
    is_price_sensitive: int

def generate_journey(device_id: str, archetype: str = None) -> ShopperJourney:
    archetypes = [
        ("quick_bounce", 0.35),
        ("casual_explorer", 0.30),
        ("price_hunter", 0.15),
        ("cart_abandoner", 0.12),
        ("decisive_buyer", 0.08),
    ]
    
    if archetype is None:
        r = random.random()
        cumulative = 0.0
        for name, weight in archetypes:
            cumulative += weight
            if r <= cumulative:
                archetype = name
                break
        if archetype is None:
            archetype = "casual_explorer"

    device_type = "mobile" if random.random() < 0.65 else "desktop"
    channels = ["direct", "organic_search", "paid_ad", "social", "referral"]
    channel_weights = [0.25, 0.35, 0.20, 0.15, 0.05]
    traffic_channel = random.choices(channels, weights=channel_weights, k=1)[0]

    if archetype == "quick_bounce":
        views = random.randint(1, 2)
        cart_adds = 0
        cart_removes = 0
        checkout_starts = 0
        distinct_prods = views
        distinct_cats = 1
        dwell = random.randint(5, 35)
        avg_scroll = round(random.uniform(0.10, 0.35), 2)
        clicks = random.randint(1, 4)
        sale_views = 0
        cart_velocity = 0.0
        has_purchase = 0
        has_abandoned = 0
        has_churned = 1
        is_price_sensitive = 0

    elif archetype == "casual_explorer":
        views = random.randint(3, 12)
        cart_adds = 1 if random.random() < 0.15 else 0
        cart_removes = 1 if cart_adds > 0 and random.random() < 0.4 else 0
        checkout_starts = 0
        distinct_prods = random.randint(2, views)
        distinct_cats = random.randint(1, min(4, distinct_prods))
        dwell = random.randint(45, 400)
        avg_scroll = round(random.uniform(0.35, 0.70), 2)
        clicks = random.randint(5, 25)
        sale_views = random.randint(0, min(2, views))
        cart_velocity = random.uniform(30.0, 180.0) if cart_adds > 0 else 0.0
        has_purchase = 0
        has_abandoned = 1 if cart_adds > cart_removes else 0
        has_churned = 1 if cart_adds == 0 else 0
        is_price_sensitive = 1 if (sale_views / max(1, views)) > 0.4 else 0

    elif archetype == "price_hunter":
        views = random.randint(5, 20)
        cart_adds = random.randint(1, 3)
        cart_removes = random.randint(0, 1)
        checkout_starts = 1 if random.random() < 0.5 else 0
        distinct_prods = random.randint(3, views)
        distinct_cats = random.randint(1, 3)
        dwell = random.randint(180, 800)
        avg_scroll = round(random.uniform(0.60, 0.95), 2)
        clicks = random.randint(15, 50)
        sale_views = random.randint(3, views)
        cart_velocity = random.uniform(60.0, 300.0)
        has_purchase = 1 if random.random() < 0.40 else 0
        has_abandoned = 1 if (has_purchase == 0 and cart_adds > 0) else 0
        has_churned = 0
        is_price_sensitive = 1

    elif archetype == "cart_abandoner":
        views = random.randint(3, 10)
        cart_adds = random.randint(1, 4)
        cart_removes = random.randint(0, 1)
        checkout_starts = 1 if random.random() < 0.8 else 0
        distinct_prods = random.randint(1, min(4, views))
        distinct_cats = random.randint(1, 2)
        dwell = random.randint(150, 900)
        avg_scroll = round(random.uniform(0.50, 0.85), 2)
        clicks = random.randint(10, 35)
        sale_views = random.randint(0, 3)
        cart_velocity = random.uniform(40.0, 200.0)
        has_purchase = 0
        has_abandoned = 1
        has_churned = 0
        is_price_sensitive = 1 if random.random() < 0.6 else 0

    else: # decisive_buyer
        views = random.randint(2, 8)
        cart_adds = random.randint(1, 3)
        cart_removes = 0
        checkout_starts = 1
        distinct_prods = random.randint(1, min(3, views))
        distinct_cats = 1
        dwell = random.randint(120, 600)
        avg_scroll = round(random.uniform(0.65, 0.95), 2)
        clicks = random.randint(8, 30)
        sale_views = random.randint(0, views)
        cart_velocity = random.uniform(15.0, 90.0)
        has_purchase = 1
        has_abandoned = 0
        has_churned = 0
        is_price_sensitive = 1 if (sale_views / max(1, views)) > 0.5 else 0

    return ShopperJourney(
        device_id=device_id,
        archetype=archetype,
        views_count=views,
        cart_adds_count=cart_adds,
        cart_removes_count=cart_removes,
        checkout_starts_count=checkout_starts,
        distinct_products=distinct_prods,
        distinct_categories=distinct_cats,
        total_dwell_seconds=dwell,
        avg_scroll_depth=avg_scroll,
        clicks_count=clicks,
        sale_item_views=sale_views,
        cart_velocity_seconds=round(cart_velocity, 2),
        device_type=device_type,
        traffic_channel=traffic_channel,
        has_purchase=has_purchase,
        has_cart_abandoned=has_abandoned,
        has_churned=has_churned,
        is_price_sensitive=is_price_sensitive,
    )

def generate_ecommerce_dataset(n_samples: int = 10000, seed: int = 42) -> List[ShopperJourney]:
    random.seed(seed)
    dataset = []
    for i in range(n_samples):
        dev_id = f"shopper-{100000 + i}"
        journey = generate_journey(dev_id)
        dataset.append(journey)
    return dataset
