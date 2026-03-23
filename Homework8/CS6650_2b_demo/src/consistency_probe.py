import json
import time
from datetime import datetime, timezone
import requests

BASE = "http://35.92.107.111:8080/shopping-carts"
observations = []

for i in range(20):
    create_resp = requests.post(BASE, json={"customer_id": 9000 + i}, timeout=10)
    cart_id = create_resp.json()["cart_id"]

    t1 = time.time()
    eventual = requests.get(f"{BASE}/{cart_id}", timeout=10)
    eventual_ms = round((time.time() - t1) * 1000, 2)

    t2 = time.time()
    strong = requests.get(f"{BASE}/{cart_id}?consistent=true", timeout=10)
    strong_ms = round((time.time() - t2) * 1000, 2)

    observations.append({
        "cart_id": cart_id,
        "eventual_status": eventual.status_code,
        "eventual_latency_ms": eventual_ms,
        "strong_status": strong.status_code,
        "strong_latency_ms": strong_ms,
        "timestamp": datetime.now(timezone.utc).isoformat()
    })

with open("dynamodb_consistency_probe.json", "w") as f:
    json.dump(observations, f, indent=2)

eventual_misses = sum(1 for x in observations if x["eventual_status"] != 200)
strong_misses = sum(1 for x in observations if x["strong_status"] != 200)

print(json.dumps({
    "iterations": len(observations),
    "eventual_misses": eventual_misses,
    "strong_misses": strong_misses
}, indent=2))
print("Saved to dynamodb_consistency_probe.json")