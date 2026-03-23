import json
from pathlib import Path

MYSQL_FILE = "mysql_results.json"
DYNAMO_FILE = "dynamodb_test_results.json"
OUT_FILE = "combined_results.json"

def load_results(path):
    data = json.loads(Path(path).read_text())
    if isinstance(data, dict) and "results" in data:
        return data["results"], data
    if isinstance(data, list):
        return data, None
    raise ValueError(f"Unsupported format in {path}")

def percentile(vals, p):
    if not vals:
        return 0.0
    vals = sorted(vals)
    k = (len(vals) - 1) * (p / 100.0)
    f = int(k)
    c = min(f + 1, len(vals) - 1)
    if f == c:
        return round(vals[f], 2)
    d0 = vals[f] * (c - k)
    d1 = vals[c] * (k - f)
    return round(d0 + d1, 2)

def normalize_op(op):
    op = op.strip().lower()
    mapping = {
        "create_cart": "CREATE_CART",
        "add_item": "ADD_ITEMS",
        "add_items": "ADD_ITEMS",
        "get_cart": "GET_CART",
        "update_item": "UPDATE_ITEM",
        "remove_item": "REMOVE_ITEM",
    }
    return mapping.get(op, op.upper())

def summarize(results):
    lat = [float(r["latency_ms"] if "latency_ms" in r else r["response_time"]) for r in results]
    success = sum(
        1
        for r in results
        if r.get("success", 200 <= int(r["status_code"]) < 300)
    )
    total = len(results)
    return {
        "total_operations": total,
        "successes": success,
        "failures": total - success,
        "success_rate_pct": round((success / total) * 100, 2) if total else 0,
        "avg_ms": round(sum(lat) / total, 2) if total else 0,
        "p50_ms": percentile(lat, 50),
        "p95_ms": percentile(lat, 95),
        "p99_ms": percentile(lat, 99),
        "latencies": lat,
    }

def counts_by_op(results):
    out = {}
    for r in results:
        op = normalize_op(r["operation"])
        out[op] = out.get(op, 0) + 1
    return out

def avg_by_op(results, op_name):
    vals = []
    for r in results:
        if normalize_op(r["operation"]) == op_name:
            vals.append(float(r["latency_ms"] if "latency_ms" in r else r["response_time"]))
    return round(sum(vals) / len(vals), 2) if vals else None

mysql_results, mysql_raw = load_results(MYSQL_FILE)
dynamo_results, dynamo_raw = load_results(DYNAMO_FILE)

mysql_summary = summarize(mysql_results)
dynamo_summary = summarize(dynamo_results)

mysql_counts = counts_by_op(mysql_results)
dynamo_counts = counts_by_op(dynamo_results)

verification = {
    "mysql_total_150": mysql_summary["total_operations"] == 150,
    "dynamodb_total_150": dynamo_summary["total_operations"] == 150,
    "mysql_expected_mix_50_50_50": mysql_counts.get("CREATE_CART", 0) == 50 and mysql_counts.get("ADD_ITEMS", 0) == 50 and mysql_counts.get("GET_CART", 0) == 50,
    "dynamodb_expected_mix_50_50_50": dynamo_counts.get("CREATE_CART", 0) == 50 and dynamo_counts.get("ADD_ITEMS", 0) == 50 and dynamo_counts.get("GET_CART", 0) == 50,
}

ops = ["CREATE_CART", "ADD_ITEMS", "GET_CART"]
op_breakdown = {}
for op in ops:
    m = avg_by_op(mysql_results, op)
    d = avg_by_op(dynamo_results, op)
    if m is None or d is None:
        faster = "N/A"
        margin = "N/A"
    elif m < d:
        faster = "MySQL"
        margin = round(d - m, 2)
    elif d < m:
        faster = "DynamoDB"
        margin = round(m - d, 2)
    else:
        faster = "Tie"
        margin = 0
    op_breakdown[op] = {
        "mysql_avg_ms": m,
        "dynamodb_avg_ms": d,
        "faster": faster,
        "margin_ms": margin,
    }

def winner(a, b, lower_better=True):
    if a == b:
        return "Tie", 0
    if lower_better:
        return ("MySQL", round(b - a, 2)) if a < b else ("DynamoDB", round(a - b, 2))
    return ("MySQL", round(a - b, 2)) if a > b else ("DynamoDB", round(b - a, 2))

avg_winner, avg_margin = winner(mysql_summary["avg_ms"], dynamo_summary["avg_ms"], True)
p50_winner, p50_margin = winner(mysql_summary["p50_ms"], dynamo_summary["p50_ms"], True)
p95_winner, p95_margin = winner(mysql_summary["p95_ms"], dynamo_summary["p95_ms"], True)
p99_winner, p99_margin = winner(mysql_summary["p99_ms"], dynamo_summary["p99_ms"], True)
sr_winner, sr_margin = winner(mysql_summary["success_rate_pct"], dynamo_summary["success_rate_pct"], False)

combined = {
    "verification": verification,
    "notes": {
        "mysql_note": "MySQL run used 30 create + 30 add + 30 get + 30 update + 30 remove in the earlier test, so it does not match the exact 50/50/50 Step III mix.",
        "dynamodb_note": "DynamoDB run matches the required 50 create + 50 add + 50 get mix.",
        "analysis_note": "Overall latency comparison uses the full recorded datasets. Operation-specific comparison focuses on CREATE_CART, ADD_ITEMS, and GET_CART only."
    },
    "mysql": {
        "summary": mysql_summary,
        "operation_counts": mysql_counts,
    },
    "dynamodb": {
        "summary": dynamo_summary,
        "operation_counts": dynamo_counts,
    },
    "comparison": {
        "performance_table": {
            "avg_response_time_ms": {
                "mysql": mysql_summary["avg_ms"],
                "dynamodb": dynamo_summary["avg_ms"],
                "winner": avg_winner,
                "margin_ms": avg_margin,
            },
            "p50_response_time_ms": {
                "mysql": mysql_summary["p50_ms"],
                "dynamodb": dynamo_summary["p50_ms"],
                "winner": p50_winner,
                "margin_ms": p50_margin,
            },
            "p95_response_time_ms": {
                "mysql": mysql_summary["p95_ms"],
                "dynamodb": dynamo_summary["p95_ms"],
                "winner": p95_winner,
                "margin_ms": p95_margin,
            },
            "p99_response_time_ms": {
                "mysql": mysql_summary["p99_ms"],
                "dynamodb": dynamo_summary["p99_ms"],
                "winner": p99_winner,
                "margin_ms": p99_margin,
            },
            "success_rate_pct": {
                "mysql": mysql_summary["success_rate_pct"],
                "dynamodb": dynamo_summary["success_rate_pct"],
                "winner": sr_winner,
                "margin_pct": sr_margin,
            },
        },
        "operation_breakdown": op_breakdown
    }
}

Path(OUT_FILE).write_text(json.dumps(combined, indent=2))

print("\nVerification:")
print(json.dumps(verification, indent=2))

print("\nPerformance Table:")
for k, v in combined["comparison"]["performance_table"].items():
    print(f"{k}: MySQL={v['mysql']}, DynamoDB={v['dynamodb']}, Winner={v['winner']}")

print("\nOperation Breakdown:")
for op, vals in op_breakdown.items():
    print(f"{op}: MySQL={vals['mysql_avg_ms']}, DynamoDB={vals['dynamodb_avg_ms']}, Faster={vals['faster']}, Margin={vals['margin_ms']}")

print(f"\nSaved {OUT_FILE}")