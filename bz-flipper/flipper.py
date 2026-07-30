#!/usr/bin/env python3
"""Standalone Hypixel SkyBlock Bazaar order-flip finder. No API key or deps beyond stdlib."""

import argparse
import json
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BAZAAR_URL = "https://api.hypixel.net/v2/skyblock/bazaar"
HOURS_PER_WEEK = 7 * 24


def fetch_products() -> dict:
    with urllib.request.urlopen(BAZAAR_URL, timeout=15) as resp:
        payload = json.load(resp)
    if not payload.get("success", True):
        raise RuntimeError(f"Hypixel API returned an error: {payload}")
    return payload["products"]


def find_flips(products: dict, undercut: float, min_margin_pct: float,
               min_volume_per_hour: float, max_orders: int) -> list[dict]:
    """Score each product as a buy-order/sell-order flip.

    Strategy: place a buy order just above the current top bid, wait for it
    to fill, then place a sell order just below the current top ask. Profit
    is the spread minus the two undercuts; this is what "bazaar flipping"
    means as distinct from crafting/insta-buy-insta-sell arbitrage.
    """
    flips = []
    for product_id, data in products.items():
        # Despite the names, buy_summary is the book of active SELL orders
        # (its top price is what you'd pay to insta-buy, i.e. the ask), and
        # sell_summary is the book of active BUY orders (its top price is
        # what you'd get insta-selling, i.e. the bid). Named for the action
        # you'd take, not who placed the order.
        sell_orders = data.get("buy_summary") or []
        buy_orders = data.get("sell_summary") or []
        status = data.get("quick_status") or {}
        if not buy_orders or not sell_orders:
            continue

        ask = sell_orders[0]["pricePerUnit"]
        bid = buy_orders[0]["pricePerUnit"]
        if bid <= 0 or ask <= 0 or ask <= bid:
            continue

        buy_order_price = bid + undercut
        sell_order_price = ask - undercut
        profit_per_item = sell_order_price - buy_order_price
        if profit_per_item <= 0:
            continue

        margin_pct = profit_per_item / buy_order_price
        if margin_pct < min_margin_pct:
            continue

        volume_per_hour = min(status.get("buyMovingWeek", 0),
                               status.get("sellMovingWeek", 0)) / HOURS_PER_WEEK
        if volume_per_hour < min_volume_per_hour:
            continue

        active_orders = status.get("buyOrders", 0) + status.get("sellOrders", 0)
        if max_orders and active_orders > max_orders:
            continue

        flips.append({
            "product_id": product_id,
            "name": product_id.replace("_", " ").title(),
            "buy_order_price": round(buy_order_price, 2),
            "sell_order_price": round(sell_order_price, 2),
            "profit_per_item": round(profit_per_item, 2),
            "margin_pct": round(margin_pct * 100, 2),
            "volume_per_hour": round(volume_per_hour, 1),
            "estimated_profit_per_hour": round(profit_per_item * volume_per_hour, 0),
            "active_orders": active_orders,
        })

    flips.sort(key=lambda f: f["estimated_profit_per_hour"], reverse=True)
    return flips


def render_table(flips: list[dict], limit: int) -> str:
    headers = ["Item", "Buy Order", "Sell Order", "Profit/Item", "Margin", "Vol/hr", "Est. Profit/hr", "Orders"]
    rows = [headers]
    for f in flips[:limit]:
        rows.append([
            f["name"],
            f"{f['buy_order_price']:,.1f}",
            f"{f['sell_order_price']:,.1f}",
            f"{f['profit_per_item']:,.1f}",
            f"{f['margin_pct']:.1f}%",
            f"{f['volume_per_hour']:,.0f}",
            f"{f['estimated_profit_per_hour']:,.0f}",
            f"{f['active_orders']:,}",
        ])
    widths = [max(len(row[i]) for row in rows) for i in range(len(headers))]
    lines = []
    for i, row in enumerate(rows):
        lines.append(" | ".join(cell.ljust(widths[j]) for j, cell in enumerate(row)))
        if i == 0:
            lines.append("-+-".join("-" * w for w in widths))
    return "\n".join(lines)


def run_once(args: argparse.Namespace) -> list[dict]:
    products = fetch_products()
    return find_flips(products, args.undercut, args.min_margin / 100,
                       args.min_volume, args.max_orders)


def serve(args: argparse.Namespace) -> None:
    cache = {"flips": [], "updated_at": 0.0, "error": None}

    def refresh():
        try:
            cache["flips"] = run_once(args)
            cache["error"] = None
        except Exception as exc:  # keep serving stale data if Hypixel hiccups
            cache["error"] = str(exc)
        cache["updated_at"] = time.time()

    refresh()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_a):
            pass

        def do_GET(self):
            if time.time() - cache["updated_at"] > args.refresh_seconds:
                refresh()

            if self.path == "/flips":
                body = json.dumps({
                    "updated_at": cache["updated_at"],
                    "error": cache["error"],
                    "flips": cache["flips"][:args.limit],
                }).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return

            age = int(time.time() - cache["updated_at"])
            table = render_table(cache["flips"], args.limit)
            error_html = f"<p style='color:red'>Last refresh error: {cache['error']}</p>" if cache["error"] else ""
            body = (
                f"<html><head><title>BZ Flipper</title></head><body>"
                f"<h1>Hypixel Bazaar Flips</h1>"
                f"<p>Updated {age}s ago. JSON: <a href='/flips'>/flips</a></p>"
                f"{error_html}<pre>{table}</pre></body></html>"
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    server = ThreadingHTTPServer(("0.0.0.0", args.port), Handler)
    print(f"Serving on http://0.0.0.0:{args.port} (refreshing every {args.refresh_seconds}s)")
    server.serve_forever()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--serve", action="store_true", help="run a small HTTP server instead of printing once")
    parser.add_argument("--port", type=int, default=8080)
    parser.add_argument("--refresh-seconds", type=float, default=30)
    parser.add_argument("--limit", type=int, default=40, help="max flips to show")
    parser.add_argument("--undercut", type=float, default=0.1, help="coins to undercut the top order by")
    parser.add_argument("--min-margin", type=float, default=1.0, help="minimum profit margin in percent")
    parser.add_argument("--min-volume", type=float, default=10.0, help="minimum traded units/hour required")
    parser.add_argument("--max-orders", type=int, default=0, help="skip items with more active orders than this (0 = no limit)")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    if args.serve:
        serve(args)
        return
    flips = run_once(args)
    if not flips:
        print("No flips matched the current filters.")
        return
    print(render_table(flips, args.limit))


if __name__ == "__main__":
    main()
