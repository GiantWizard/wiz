# wiz

A Hypixel SkyBlock Bazaar analysis pipeline: poll the live bazaar API, track order-flow
metrics over time, and run a profit optimizer over crafting/fusion recipe chains to find
worthwhile flips. Originally built in 2024, then reworked and cleaned up later.

## Status

This is a finished side project, not an actively maintained one. The core engine works and
is tested against live data (see "Running it" below); some of the surrounding pieces
(frontend, side-experiment bots) are lighter-weight and less polished.

## How it's structured

- **`backend/calculation_engine/`** (Go) - the actual analysis engine. Loads bazaar order-flow
  metrics, polls the live Hypixel bazaar API, expands crafting/fusion recipes (from
  `dependencies/items/`, ~7,400 Hypixel item definitions), and runs a profit optimizer that
  finds the most profitable feasible quantity to flip for each item under a time constraint.
  Serves results over HTTP (`/optimizer_results.json`, `/status`, `/healthz`). Has unit tests
  for the core math and an integration test that runs the optimizer against live bazaar data.
- **`backend/server9/`** (Rust + C++) - the metrics producer. Polls the Hypixel bazaar API,
  computes the order-flow statistics (pattern detection, moving averages) that the calculation
  engine consumes, and uploads snapshots to MEGA.
- **`backend/Dockerfile`** / **`supervisord.conf`** - builds and runs server9, the calculation
  engine, and a MEGA session manager together as one deployable image. MEGA is the transport
  between the two: server9 uploads metrics snapshots, the calculation engine downloads them.
  Needs a MEGA account's credentials (`MEGA_EMAIL`/`MEGA_PWD`) to actually move data between
  them; the calculation engine still works standalone off its last-known-good local snapshot
  and live bazaar data if MEGA is unreachable.
- **`frontend/`** (SvelteKit) - dashboard for browsing computed optimizer results. Proxies to
  the calculation engine's HTTP API (`CALCULATION_ENGINE_URL` env var, defaults to
  `localhost:9000` for local dev). A couple of secondary chart features (auction-house lowest-bin
  and multi-day price history) reference data files no producer in this repo ever generates -
  those charts won't populate.
- **`fusion_dashboard/`** (Python/Flask) - separate, self-contained tool that prices out
  SkyBlock fusion recipes against live bazaar prices.
- **`server10/`** (Python/Flask) - separate, self-contained flipper: tracks which items have
  stable order flow and flags high-margin instabuy/instasell spreads among them.
- **`bot-test/sky-freaky/`** - a minimal Discord Cloudflare Worker (ping/pong slash command
  scaffold). Needs `DISCORD_TOKEN`, `DISCORD_PUBLIC_KEY`, `DISCORD_APPLICATION_ID` as Worker
  secrets to actually run as a bot.
- **`info/`** - two real historical metrics snapshots (start and end of a June 2025 tracking
  run), kept as fixture data.

## Running it

The calculation engine runs standalone with no external accounts needed - it ships with a
real historical metrics snapshot and talks to Hypixel's public (keyless) bazaar API:

```
cd backend/calculation_engine
go test ./...     # unit tests + a live-data integration test
go run .          # starts on :9000, serves /optimizer_results.json after ~30s
```

To see it in the dashboard:

```
cd frontend
npm install
CALCULATION_ENGINE_URL=http://localhost:9000 npm run dev
```

Running the full producer -> MEGA -> consumer chain (`backend/Dockerfile`) additionally
requires a MEGA account and `server9`'s Rust/C++ toolchain.
