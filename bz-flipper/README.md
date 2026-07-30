# bz-flipper

A small, dependency-free (stdlib only) Hypixel SkyBlock Bazaar order-flipping
tool. It's separate from the rest of this repo's pipeline (`backend/`,
`server10/`, `fusion_dashboard/`) — no MEGA, no metrics history, no build
step, just one Python file that talks directly to Hypixel's public bazaar
endpoint.

## Background: how Bazaar flipping actually works

Hypixel's bazaar endpoint (`GET https://api.hypixel.net/v2/skyblock/bazaar`,
no API key required) returns, per item:

- `buy_summary`: the active **buy orders** (bids) for that item, highest
  price first. The top entry is the best price you'd get if you instantly
  sold into the book.
- `sell_summary`: the active **sell orders** (asks) for that item, lowest
  price first. The top entry is the best price you'd pay if you instantly
  bought from the book.
- `quick_status`: aggregate stats — `buyPrice`/`sellPrice` (weighted
  averages of the summaries above), `buyOrders`/`sellOrders` (how many
  distinct orders are sitting in each book — a proxy for how much
  competition/undercutting you'll face), and `buyMovingWeek`/
  `sellMovingWeek` (units transacted via each side over the last 7 days —
  the best available proxy for how fast an order will actually fill).

Since `ask > bid` normally (that gap is what market makers earn), the
profitable play isn't to insta-buy then insta-sell — that loses money. It's
to **place your own buy order slightly above the current top bid**, wait
for someone to sell into it, then **place a sell order slightly below the
current top ask**, waiting for someone to buy it. You collect the spread
minus the two small undercuts. That's what this tool ranks: it is not a
crafting/insta-flip arbitrage tool (see `backend/calculation_engine` for
that instead).

## What the script does

For every product it computes:

- `buy_order_price` / `sell_order_price` — top bid/ask nudged by
  `--undercut` coins, i.e. the orders you'd actually place.
- `profit_per_item` — the spread you'd capture per unit after undercutting.
- `margin_pct` — that profit as a percentage of your buy-order price.
- `volume_per_hour` — `min(buyMovingWeek, sellMovingWeek) / (7*24)`, a
  liquidity floor: an item can't fill faster than the slower side of its
  own market actually trades.
- `estimated_profit_per_hour` — `profit_per_item * volume_per_hour`. This
  assumes you capture *all* of the hourly volume, which is optimistic on
  crowded items (see `active_orders`) and conservative on thin ones where
  a single large order could fill instantly — treat it as a ranking
  signal, not a guarantee.

Results are filtered by `--min-margin`, `--min-volume`, and optionally
`--max-orders` (skip items with too much order-book competition to
realistically stay at the top), then sorted by estimated profit/hour.

## Running it

```
python3 flipper.py                 # one-shot, prints a table to stdout
python3 flipper.py --min-margin 2 --min-volume 50
python3 flipper.py --serve --port 8080   # background refresh + web UI at / and JSON at /flips
```

No install step — it only uses the Python standard library. Requires
outbound HTTPS access to `api.hypixel.net`.

Flags (`python3 flipper.py --help`):

| Flag | Default | Meaning |
|---|---|---|
| `--undercut` | `0.1` | Coins to undercut the top order by on each side |
| `--min-margin` | `1.0` | Minimum profit margin, in percent |
| `--min-volume` | `10.0` | Minimum units/hour traded on the thinner side |
| `--max-orders` | `0` (off) | Skip items with more active orders than this |
| `--limit` | `40` | Max rows shown |
| `--serve` | off | Run an HTTP server instead of a single run |
| `--refresh-seconds` | `30` | How often `--serve` re-polls Hypixel |

## Caveats

- This ranks by a simplified liquidity model; it doesn't simulate order
  queue position, so a crowded book (`active_orders` high) will fill
  slower than the estimate implies.
- Bazaar currently has no transaction tax, so none is modeled. If Hypixel
  reintroduces one, subtract it from `profit_per_item` in `find_flips`.
- Not tested against live data in this session — outbound network access
  to `api.hypixel.net` isn't available in this environment. The bazaar
  response shape is stable and well documented (and matches the fields
  already used in `server10/fetchur.py` and `backend/calculation_engine`
  elsewhere in this repo), so pull the branch and run it locally to see
  live results.
