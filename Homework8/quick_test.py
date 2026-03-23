import json
import time
import requests
from statistics import mean, median

BASE = "http://35.160.77.222:8080"

results = []
op_counts = {
    "create_cart": 0,
    "get_cart": 0,
    "add_item": 0,
    "update_item": 0,
    "remove_item": 0,
}

cart_ids = []

def record(op, start, resp):
    elapsed_ms = (time.time() - start) * 1000
    results.append({
        "operation": op,
        "status_code": resp.status_code,
        "latency_ms": round(elapsed_ms, 2)
    })

for i in range(30):
    start = time.time()
    resp = requests.post(f"{BASE}/carts", json={"customer_id": 1000 + i})
    record("create_cart", start, resp)
    op_counts["create_cart"] += 1
    if resp.ok:
        try:
            cart_ids.append(resp.json()["cart_id"])
        except Exception:
            pass

for i, cart_id in enumerate(cart_ids):
    start = time.time()
    resp = requests.post(
        f"{BASE}/carts/{cart_id}/items",
        json={
            "product_id": 2000 + i,
            "product_name": f"Item-{i}",
            "unit_price": 10.0 + i,
            "quantity": 1
        }
    )
    record("add_item", start, resp)
    op_counts["add_item"] += 1

for cart_id in cart_ids:
    start = time.time()
    resp = requests.get(f"{BASE}/carts/{cart_id}")
    record("get_cart", start, resp)
    op_counts["get_cart"] += 1

for i, cart_id in enumerate(cart_ids):
    start = time.time()
    resp = requests.put(
        f"{BASE}/carts/{cart_id}/items/{2000 + i}",
        json={"quantity": 3}
    )
    record("update_item", start, resp)
    op_counts["update_item"] += 1

for i, cart_id in enumerate(cart_ids):
    start = time.time()
    resp = requests.delete(f"{BASE}/carts/{cart_id}/items/{2000 + i}")
    record("remove_item", start, resp)
    op_counts["remove_item"] += 1

latencies = [r["latency_ms"] for r in results]
successes = sum(1 for r in results if 200 <= r["status_code"] < 300)
failures = len(results) - successes

summary = {
    "total_operations": len(results),
    "successes": successes,
    "failures": failures,
    "mean_latency_ms": round(mean(latencies), 2) if latencies else 0,
    "median_latency_ms": round(median(latencies), 2) if latencies else 0,
    "min_latency_ms": round(min(latencies), 2) if latencies else 0,
    "max_latency_ms": round(max(latencies), 2) if latencies else 0,
    "operation_counts": op_counts,
    "results": results
}

with open("mysql_results.json", "w") as f:
    json.dump(summary, f, indent=2)

print(json.dumps(summary, indent=2))