import json
import time
from datetime import datetime, timezone
import requests

BASE = "http://35.92.107.111:8080/shopping-carts"

results = []
cart_ids = []

def record(operation, start_time, response):
    elapsed_ms = round((time.time() - start_time) * 1000, 2)
    results.append({
        "operation": operation,
        "response_time": elapsed_ms,
        "success": 200 <= response.status_code < 300,
        "status_code": response.status_code,
        "timestamp": datetime.now(timezone.utc).isoformat()
    })

# 50 create
for i in range(50):
    start = time.time()
    resp = requests.post(BASE, json={"customer_id": 1000 + i}, timeout=10)
    record("create_cart", start, resp)
    if 200 <= resp.status_code < 300:
        cart_ids.append(resp.json()["cart_id"])

# 50 add
for i, cart_id in enumerate(cart_ids[:50]):
    start = time.time()
    resp = requests.post(
        f"{BASE}/{cart_id}/items",
        json={
            "product_id": 5000 + i,
            "product_name": f"Item-{i}",
            "unit_price": 10.99 + i,
            "quantity": 1
        },
        timeout=10
    )
    record("add_items", start, resp)

# 50 get
for cart_id in cart_ids[:50]:
    start = time.time()
    resp = requests.get(f"{BASE}/{cart_id}", timeout=10)
    record("get_cart", start, resp)

with open("dynamodb_test_results.json", "w") as f:
    json.dump(results, f, indent=2)

successes = sum(1 for r in results if r["success"])
failures = len(results) - successes
times = [r["response_time"] for r in results]

summary = {
    "total_operations": len(results),
    "successes": successes,
    "failures": failures,
    "mean_response_time_ms": round(sum(times) / len(times), 2) if times else 0,
    "min_response_time_ms": round(min(times), 2) if times else 0,
    "max_response_time_ms": round(max(times), 2) if times else 0
}

print(json.dumps(summary, indent=2))
print("Saved to dynamodb_test_results.json")