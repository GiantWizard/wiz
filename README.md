# wiz

A Hypixel SkyBlock Bazaar analysis pipeline: poll the live bazaar API, track order-flow metrics over time, and run a profit optimizer over crafting/fusion recipe chains to find worthwhile flips. Originally built in 2024, then reworked and cleaned up later.

## Status

This is a finished side project. The core engine works (or used to) and has been tested against live data; not everything is very polished as this was made a long time ago when I didn't know anything about the development process.

## How it's structured

- **`backend/calculation_engine/`** (Go) - This is the actual analysis engine. It loads bazaar metrics, polls the live Hypixel bazaar API, expands crafting/fusion recipes (from `dependencies/items/`, containing data of ~7,400 Hypixel items), and runs a profit optimizer that finds the most profitable feasible quantity of items to flip for profit.
- **`backend/server9/`** (Rust + C++) - This component continuously polls the Hypixel Bazaar API, then computes metrics that the calculation engine consumes, then uploads these snapshots to MEGA. Rust is used to save memory, while C++ is used because MEGA only lets you use C++ in the CLI.
- **`backend/Dockerfile`** / **`supervisord.conf`** - Builds and runs server9, the calculation engine, and a MEGA session manager together. MEGA acts as a transport between the two: server9 uploads metrics snapshots, the calculation engine downloads and processes them.
- **`frontend/`** (SvelteKit) - Dashboard for browsing computed optimizer results. Proxies to the calculation engine's HTTP API (`CALCULATION_ENGINE_URL` env var, defaults to `localhost:9000` for local dev). Has a couple of secondary chart features (auction-house lowest BIN and price history) reference data files this repo doesn't use.
- **`fusion_dashboard/`** (Python/Flask) - Separate tool that checks SkyBlock fusion recipes against live bazaar prices.
- **`server10/`** (Python/Flask) - Another self contained flipper tool that's a little different from `server9/`
- **`bot-test/sky-freaky/`** - Minimal Discord Cloudflare Worker, used to hold fusion flipping algorithm. Needs `DISCORD_TOKEN`, `DISCORD_PUBLIC_KEY`, `DISCORD_APPLICATION_ID` as Worker secrets to actually run as a bot.
- **`info/`** - two old historical metrics snapshots (start and end of a June 2025 tracking run), kept in case someone wants to see check it out.

## Running it

The calculation engine runs on it's own with no external accounts needed - it ships with a real historical metrics snapshot and talks to Hypixel's (keyless) bazaar API:

```
cd backend/calculation_engine
go test ./...     # unit tests
go run .          # starts on :9000, serves /optimizer_results.json after ~30s
```

To see it in the dashboard:

```
cd frontend
npm install
CALCULATION_ENGINE_URL=http://localhost:9000 npm run dev
```

Running the full producer -> MEGA -> consumer chain (`backend/Dockerfile`) additionally requires a MEGA account and `server9`'s Rust/C++ toolchain.
