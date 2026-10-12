#!/usr/bin/env python3
"""
stress_test_local.py - Run concurrent copy-trade stress test on local EnvoyTrade deployment,
isolate newly created test orders, calculate P50, P90, P99 latency percentiles,
and archive results to ../EnvoyBenchmarkData/11th_Oct_latency_data_local.
"""

import sys
import os
import argparse
import time
import json
import csv
import math
import subprocess
import concurrent.futures
import urllib.request
import urllib.parse

TESTBROKER_URL = "http://localhost:8089"
LOCAL_DB_URL = "postgres://envoytrade:envoytrade@localhost:5434/envoytrade?sslmode=disable"
DEFAULT_OUT_DIR = "../EnvoyBenchmarkData/11th_Oct_latency_data_local"

def percentile(values, p):
    if not values:
        return 0.0
    k = (len(values) - 1) * (p / 100.0)
    f = math.floor(k)
    c = math.ceil(k)
    if f == c:
        return values[int(k)]
    d0 = values[int(f)] * (c - k)
    d1 = values[int(c)] * (k - f)
    return d0 + d1

def compute_stats(values):
    clean = sorted([float(v) for v in values if v is not None and not math.isnan(float(v))])
    if not clean:
        return {
            "count": 0, "min": 0.0, "avg": 0.0, "p50": 0.0, 
            "p90": 0.0, "p95": 0.0, "p99": 0.0, "max": 0.0
        }
    return {
        "count": len(clean),
        "min": clean[0],
        "avg": sum(clean) / len(clean),
        "p50": percentile(clean, 50),
        "p90": percentile(clean, 90),
        "p95": percentile(clean, 95),
        "p99": percentile(clean, 99),
        "max": clean[-1]
    }

def get_current_watermarks(db_url):
    query = "SELECT COALESCE(MAX(id), 0) FROM master_fills; SELECT COALESCE(MAX(id), 0) FROM follower_orders;"
    cmd = ["psql", db_url, "-t", "-A", "-c", query]
    res = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=True)
    lines = [line.strip() for line in res.stdout.strip().splitlines() if line.strip()]
    max_master_id = int(lines[0])
    max_follower_id = int(lines[1])
    return max_master_id, max_follower_id

def place_master_order(user, exchange, symbol, qty, transaction_type="BUY", order_type="MARKET", product="CNC"):
    url = f"{TESTBROKER_URL}/orders/regular"
    api_key = f"key_{user.lower()}"
    token = f"token_{user.lower()}"
    headers = {
        "Authorization": f"token {api_key}:{token}",
        "Content-Type": "application/x-www-form-urlencoded"
    }
    data = urllib.parse.urlencode({
        "exchange": exchange,
        "tradingsymbol": symbol,
        "transaction_type": transaction_type,
        "order_type": order_type,
        "quantity": qty,
        "product": product
    }).encode("utf-8")
    
    req = urllib.request.Request(url, data=data, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            body = json.loads(resp.read().decode())
            return True, body.get("data", {}).get("order_id"), None
    except Exception as e:
        return False, None, str(e)

def run_stress_burst(burst_count=20):
    print(f"\n--> Starting concurrent stress burst of {burst_count} master orders...")
    tasks = []
    
    symbols = [
        ("MASTER01", "NSE", "RELIANCE", 2, "CNC"),
        ("MASTER01", "NSE", "INFY", 5, "CNC"),
        ("MASTER01", "MCX", "CRUDEOIL21SEP26", 100, "MIS"),
        ("MASTER02", "NSE", "RELIANCE", 4, "CNC"),
        ("MASTER02", "NSE", "INFY", 10, "CNC"),
        ("MASTER02", "MCX", "CRUDEOIL21SEP26", 100, "MIS"),
    ]
    
    start_time = time.time()
    results = []
    
    with concurrent.futures.ThreadPoolExecutor(max_workers=10) as executor:
        for i in range(burst_count):
            master_user, exchange, symbol, qty, product = symbols[i % len(symbols)]
            # alternate BUY/SELL to exercise diverse order flows
            tx_type = "BUY" if i % 2 == 0 else "SELL"
            tasks.append(executor.submit(
                place_master_order, master_user, exchange, symbol, qty, tx_type, "MARKET", product
            ))
        
        for future in concurrent.futures.as_completed(tasks):
            success, order_id, err = future.result()
            results.append((success, order_id, err))

    duration = time.time() - start_time
    successful_orders = [r[1] for r in results if r[0]]
    failed_orders = [r[2] for r in results if not r[0]]
    
    print(f"--> Placed {len(successful_orders)}/{burst_count} master orders in {duration:.3f}s (Failed: {len(failed_orders)})")
    if failed_orders:
        print(f"    Sample error: {failed_orders[0]}")
    return successful_orders

def fetch_test_order_latencies(db_url, start_master_id, start_follower_id):
    query = f"""
    SELECT json_agg(t) FROM (
        SELECT 
            fo.id AS follower_order_id,
            fo.master_fill_id,
            mf.broker_order_id AS master_broker_order_id,
            fo.broker_order_id AS follower_broker_order_id,
            fo.tradingsymbol,
            fo.exchange,
            fo.terminal_status,
            fo.placed_qty,
            
            mf.order_timestamp AS master_fill_time,
            mf.received_at AS master_received_at,
            mf.dispatched_at AS master_dispatched_at,
            fo.created_at AS follower_created_at,
            fo.updated_at AS follower_settled_at,
            
            oe_resp.occurred_at AS follower_placed_at,
            
            -- Stage 1: Detection / Ingestion lag (ms)
            ROUND(EXTRACT(EPOCH FROM (mf.received_at - mf.order_timestamp)) * 1000, 2) AS detection_lag_ms,
            
            -- Stage 2: Engine sizing and fan-out latency (ms)
            ROUND(EXTRACT(EPOCH FROM (fo.created_at - mf.received_at)) * 1000, 2) AS engine_fanout_ms,
            
            -- Stage 3: Follower broker placement roundtrip (ms)
            ROUND(EXTRACT(EPOCH FROM (oe_resp.occurred_at - fo.created_at)) * 1000, 2) AS broker_placement_ms,
            
            -- Stage 4: Platform latency (Received master fill -> Follower order placed) (ms)
            ROUND(EXTRACT(EPOCH FROM (COALESCE(oe_resp.occurred_at, fo.updated_at) - mf.received_at)) * 1000, 2) AS platform_latency_ms,
            
            -- Stage 5: Total end-to-end latency (Broker master fill -> Follower settlement) (ms)
            ROUND(EXTRACT(EPOCH FROM (fo.updated_at - mf.order_timestamp)) * 1000, 2) AS total_e2e_ms

        FROM follower_orders fo
        JOIN master_fills mf ON fo.master_fill_id = mf.id
        LEFT JOIN LATERAL (
            SELECT occurred_at 
            FROM order_events 
            WHERE follower_order_id = fo.id AND event_type = 'api_response'
            ORDER BY id ASC 
            LIMIT 1
        ) oe_resp ON true
        WHERE fo.id > {start_follower_id} AND mf.id > {start_master_id}
        ORDER BY fo.id ASC
    ) t;
    """.strip()
    
    cmd = ["psql", db_url, "-t", "-A", "-c", query]
    res = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=True)
    raw = res.stdout.strip()
    if not raw or raw == "null":
        return []
    return json.loads(raw)

def main():
    parser = argparse.ArgumentParser(description="Local EnvoyTrade Stress Test & Latency Benchmark")
    parser.add_argument("--burst", type=int, default=24, help="Number of concurrent master orders to trigger (default: 24)")
    parser.add_argument("--db-url", default=LOCAL_DB_URL, help="PostgreSQL connection string")
    parser.add_argument("--out-dir", default=DEFAULT_OUT_DIR, help="Destination directory for benchmark data")
    args = parser.parse_args()

    os.makedirs(args.out_dir, exist_ok=True)
    
    print("==================================================================")
    print(" EnvoyTrade Local Deployment Latency Stress Benchmark")
    print(f" TestBroker: {TESTBROKER_URL}")
    print(f" Database:   {args.db_url}")
    print(f" Output Dir: {os.path.abspath(args.out_dir)}")
    print("==================================================================")

    # 1. Capture Pre-Test Watermarks
    start_master_id, start_follower_id = get_current_watermarks(args.db_url)
    print(f"--> Baseline Watermarks: Max Master ID = {start_master_id}, Max Follower ID = {start_follower_id}")

    # 2. Fire Concurrent Burst
    orders = run_stress_burst(burst_count=args.burst)
    if not orders:
        print("ERROR: No orders placed successfully. Aborting.", file=sys.stderr)
        sys.exit(1)

    # 3. Allow brief settle time for fan-out and DB settlement
    print("--> Waiting 4 seconds for all orders to settle through engine & workers...")
    time.sleep(4)

    # 4. Fetch Isolated Latency Records
    records = fetch_test_order_latencies(args.db_url, start_master_id, start_follower_id)
    print(f"--> Isolated {len(records)} follower copy-trade orders triggered strictly during this test.")

    if not records:
        print("WARNING: No follower orders detected for test run. Check follow_links configuration.")
        return

    # 5. Save Raw CSV & JSON
    csv_file = os.path.join(args.out_dir, "copy_trade_latency_local.csv")
    json_file = os.path.join(args.out_dir, "copy_trade_latency_local.json")

    with open(json_file, "w", encoding="utf-8") as f:
        json.dump(records, f, indent=2, default=str)
    print(f"--> Saved local raw JSON to: {json_file}")

    fieldnames = list(records[0].keys())
    with open(csv_file, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        for r in records:
            writer.writerow(r)
    print(f"--> Saved local raw CSV to:  {csv_file}")

    # 6. Compute Statistics
    detection_lags = [r.get("detection_lag_ms") for r in records if r.get("detection_lag_ms") is not None]
    engine_fanouts = [r.get("engine_fanout_ms") for r in records if r.get("engine_fanout_ms") is not None]
    broker_placements = [r.get("broker_placement_ms") for r in records if r.get("broker_placement_ms") is not None]
    platform_latencies = [r.get("platform_latency_ms") for r in records if r.get("platform_latency_ms") is not None]
    total_e2es = [r.get("total_e2e_ms") for r in records if r.get("total_e2e_ms") is not None]

    stats = {
        "detection_lag_broker_to_envoytrade": compute_stats(detection_lags),
        "engine_sizing_and_fanout_latency": compute_stats(engine_fanouts),
        "follower_broker_placement_roundtrip": compute_stats(broker_placements),
        "total_platform_latency_ingest_to_placed": compute_stats(platform_latencies),
        "full_end_to_end_latency_master_fill_to_settlement": compute_stats(total_e2es),
    }

    # 7. Save Metrics Summary JSON
    summary_file = os.path.join(args.out_dir, "latency_metrics_summary.json")
    summary_payload = {
        "benchmark_type": "local_deployment_stress_test",
        "benchmark_date": time.strftime("%Y-%m-%d %H:%M:%S"),
        "testbroker_url": TESTBROKER_URL,
        "database_url": args.db_url,
        "master_orders_fired": len(orders),
        "follower_orders_evaluated": len(records),
        "baseline_watermarks": {
            "initial_master_fill_id": start_master_id,
            "initial_follower_order_id": start_follower_id
        },
        "metrics": stats
    }
    with open(summary_file, "w", encoding="utf-8") as f:
        json.dump(summary_payload, f, indent=2)
    print(f"--> Saved metrics summary JSON to: {summary_file}")

    # 8. Print Formatted Benchmark Table
    print("\n" + "=" * 94)
    print(f"             ENVOYTRADE LOCAL STRESS TEST LATENCY BENCHMARK REPORT")
    print(f"             Target: Localhost | Master Orders: {len(orders)} | Follower Copies: {len(records)}")
    print("=" * 94)
    header = f"{'Latency Metric Stage':<42} | {'Count':<6} | {'Min (ms)':<9} | {'Avg (ms)':<9} | {'P50 (ms)':<9} | {'P90 (ms)':<9} | {'P95 (ms)':<9} | {'P99 (ms)':<9}"
    print(header)
    print("-" * 94)

    display_names = {
        "detection_lag_broker_to_envoytrade": "Detection Lag (Broker -> EnvoyTrade)",
        "engine_sizing_and_fanout_latency": "Engine Sizing & Fan-Out Latency",
        "follower_broker_placement_roundtrip": "Broker Placement Roundtrip",
        "total_platform_latency_ingest_to_placed": "Total Platform Latency (Ingest -> Placed)",
        "full_end_to_end_latency_master_fill_to_settlement": "Full End-to-End (Master Fill -> Settle)",
    }

    for key, s in stats.items():
        name = display_names.get(key, key)
        row = (
            f"{name:<42} | "
            f"{s['count']:<6} | "
            f"{s['min']:<9.2f} | "
            f"{s['avg']:<9.2f} | "
            f"{s['p50']:<9.2f} | "
            f"{s['p90']:<9.2f} | "
            f"{s['p95']:<9.2f} | "
            f"{s['p99']:<9.2f}"
        )
        print(row)

    print("=" * 94)

    # 9. Write README.md
    readme_file = os.path.join(args.out_dir, "README.md")
    with open(readme_file, "w", encoding="utf-8") as f:
        f.write(f"""# EnvoyTrade Local Stress Test Benchmark Report

- **Date of Test**: {time.strftime('%Y-%m-%d %H:%M:%S')}
- **Target Deployment**: Localhost (EnvoyTrade `{TESTBROKER_URL}` + Postgres on `5434`)
- **Master Orders Triggered**: {len(orders)} concurrent orders
- **Follower Copy-Trades Evaluated**: {len(records)} copy orders strictly triggered during this run
- **Baseline Watermarks**: MasterFill ID > `{start_master_id}`, FollowerOrder ID > `{start_follower_id}`

---

## 1. Latency Percentiles Breakdown

All times in milliseconds (ms).

| Stage | Samples | Min (ms) | Avg (ms) | P50 (ms) | P90 (ms) | P95 (ms) | P99 (ms) |
|---|---|---|---|---|---|---|---|
| **1. Detection Lag** *(Broker Fill $\\rightarrow$ Ingest)* | {stats['detection_lag_broker_to_envoytrade']['count']} | {stats['detection_lag_broker_to_envoytrade']['min']:.2f} | {stats['detection_lag_broker_to_envoytrade']['avg']:.2f} | **{stats['detection_lag_broker_to_envoytrade']['p50']:.2f}** | **{stats['detection_lag_broker_to_envoytrade']['p90']:.2f}** | {stats['detection_lag_broker_to_envoytrade']['p95']:.2f} | {stats['detection_lag_broker_to_envoytrade']['p99']:.2f} |
| **2. Engine Sizing & Fan-Out** *(Internal)* | {stats['engine_sizing_and_fanout_latency']['count']} | {stats['engine_sizing_and_fanout_latency']['min']:.2f} | {stats['engine_sizing_and_fanout_latency']['avg']:.2f} | **{stats['engine_sizing_and_fanout_latency']['p50']:.2f}** | **{stats['engine_sizing_and_fanout_latency']['p90']:.2f}** | {stats['engine_sizing_and_fanout_latency']['p95']:.2f} | **{stats['engine_sizing_and_fanout_latency']['p99']:.2f}** |
| **3. Broker Placement Roundtrip** *(Network/Mock)* | {stats['follower_broker_placement_roundtrip']['count']} | {stats['follower_broker_placement_roundtrip']['min']:.2f} | {stats['follower_broker_placement_roundtrip']['avg']:.2f} | **{stats['follower_broker_placement_roundtrip']['p50']:.2f}** | **{stats['follower_broker_placement_roundtrip']['p90']:.2f}** | {stats['follower_broker_placement_roundtrip']['p95']:.2f} | **{stats['follower_broker_placement_roundtrip']['p99']:.2f}** |
| **4. Total Platform Latency** *(Ingest $\\rightarrow$ Placed)* | {stats['total_platform_latency_ingest_to_placed']['count']} | {stats['total_platform_latency_ingest_to_placed']['min']:.2f} | {stats['total_platform_latency_ingest_to_placed']['avg']:.2f} | **{stats['total_platform_latency_ingest_to_placed']['p50']:.2f}** | **{stats['total_platform_latency_ingest_to_placed']['p90']:.2f}** | {stats['total_platform_latency_ingest_to_placed']['p95']:.2f} | **{stats['total_platform_latency_ingest_to_placed']['p99']:.2f}** |
| **5. Full End-to-End Latency** *(Master Fill $\\rightarrow$ Settlement)* | {stats['full_end_to_end_latency_master_fill_to_settlement']['count']} | {stats['full_end_to_end_latency_master_fill_to_settlement']['min']:.2f} | {stats['full_end_to_end_latency_master_fill_to_settlement']['avg']:.2f} | **{stats['full_end_to_end_latency_master_fill_to_settlement']['p50']:.2f}** | **{stats['full_end_to_end_latency_master_fill_to_settlement']['p90']:.2f}** | {stats['full_end_to_end_latency_master_fill_to_settlement']['p95']:.2f} | **{stats['full_end_to_end_latency_master_fill_to_settlement']['p99']:.2f}** |

---

## 2. Included Data Files

- [`copy_trade_latency_local.csv`](./copy_trade_latency_local.csv): Raw CSV export of the {len(records)} test orders.
- [`copy_trade_latency_local.json`](./copy_trade_latency_local.json): Raw JSON export of test orders.
- [`latency_metrics_summary.json`](./latency_metrics_summary.json): Statistical metrics summary JSON file.
""")
    print(f"--> Saved local summary report to: {readme_file}\n")

if __name__ == "__main__":
    main()
