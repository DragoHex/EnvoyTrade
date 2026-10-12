#!/usr/bin/env python3
"""
analyze_copy_latency.py - Fetch copy-trade latency data from deployed PostgreSQL
and calculate P50, P90, P99 percentiles across copy-trade stages.

STRICTLY READ-ONLY: Executes only SELECT statements against the database.
"""

import sys
import os
import argparse
import json
import csv
import subprocess
import math

REMOTE_DEFAULT = "envoytrade@srv2036476.hstgr.cloud"
PG_CONN_DEFAULT = "postgres://envoytrade:envoytrade@/envoytrade?host=/var/run/postgresql"

SQL_QUERY_TEMPLATE = """
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
    WHERE fo.created_at >= NOW() - INTERVAL '{days} days'
    ORDER BY fo.created_at DESC
) t;
"""

def percentile(values, p):
    """Calculates percentile using standard nearest-rank / linear interpolation."""
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

def fetch_data(remote_target, pg_conn, days):
    query = SQL_QUERY_TEMPLATE.format(days=int(days)).strip()
    # Execute safely via SSH with read-only query
    remote_cmd = f"psql '{pg_conn}' -t -A -c \"{query}\""
    cmd = ["ssh", remote_target, remote_cmd]
    print(f"--> Connecting to {remote_target} (Read-Only) ...")
    print(f"--> Fetching copy-trade records for the last {days} days ...")
    
    res = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    if res.returncode != 0:
        print(f"Error querying database via SSH: {res.stderr}", file=sys.stderr)
        sys.exit(1)
        
    raw_output = res.stdout.strip()
    if not raw_output or raw_output == "null":
        print(f"No records found for the last {days} days.")
        return []
    
    try:
        data = json.loads(raw_output)
        return data or []
    except json.JSONDecodeError as e:
        print(f"Failed to parse database JSON response: {e}\nRaw output: {raw_output[:500]}", file=sys.stderr)
        sys.exit(1)

def main():
    parser = argparse.ArgumentParser(description="EnvoyTrade Copy-Trade Latency Benchmark & Extractor")
    parser.add_argument("--days", type=int, default=7, help="Number of past days to query (default: 7)")
    parser.add_argument("--remote-host", default=REMOTE_DEFAULT, help="SSH remote user@host")
    parser.add_argument("--db-url", default=PG_CONN_DEFAULT, help="Postgres connection string")
    parser.add_argument("--out-dir", default="./reports", help="Output directory for saved files")
    args = parser.parse_args()

    os.makedirs(args.out_dir, exist_ok=True)
    records = fetch_data(args.remote_host, args.db_url, args.days)
    
    print(f"--> Successfully retrieved {len(records)} copy-trade records.")

    # 1. Save Raw Data
    csv_file = os.path.join(args.out_dir, f"copy_trade_latency_last_{args.days}d.csv")
    json_file = os.path.join(args.out_dir, f"copy_trade_latency_last_{args.days}d.json")

    with open(json_file, "w", encoding="utf-8") as f:
        json.dump(records, f, indent=2, default=str)
    print(f"--> Saved raw JSON data to: {json_file}")

    if records:
        fieldnames = list(records[0].keys())
        with open(csv_file, "w", newline="", encoding="utf-8") as f:
            writer = csv.DictWriter(f, fieldnames=fieldnames)
            writer.writeheader()
            for r in records:
                writer.writerow(r)
        print(f"--> Saved raw CSV data to:  {csv_file}")

    if not records:
        return

    # 2. Extract series
    detection_lags = [r.get("detection_lag_ms") for r in records if r.get("detection_lag_ms") is not None]
    engine_fanouts = [r.get("engine_fanout_ms") for r in records if r.get("engine_fanout_ms") is not None]
    broker_placements = [r.get("broker_placement_ms") for r in records if r.get("broker_placement_ms") is not None]
    platform_latencies = [r.get("platform_latency_ms") for r in records if r.get("platform_latency_ms") is not None]
    total_e2es = [r.get("total_e2e_ms") for r in records if r.get("total_e2e_ms") is not None]

    stats = {
        "Detection Lag (Broker -> EnvoyTrade)": compute_stats(detection_lags),
        "Engine Sizing & Fan-Out Latency": compute_stats(engine_fanouts),
        "Broker Placement Roundtrip": compute_stats(broker_placements),
        "Total Platform Latency (Ingest -> Placed)": compute_stats(platform_latencies),
        "Full End-to-End Latency (Master Fill -> Settlement)": compute_stats(total_e2es),
    }

    # 3. Print Report
    print("\n" + "=" * 94)
    print(f"             ENVOYTRADE COPY-TRADE LATENCY BENCHMARK REPORT (LAST {args.days} DAYS)")
    print(f"             Host: {args.remote_host} | Total Trades: {len(records)}")
    print("=" * 94)
    header = f"{'Latency Metric Stage':<42} | {'Count':<6} | {'Min (ms)':<9} | {'Avg (ms)':<9} | {'P50 (ms)':<9} | {'P90 (ms)':<9} | {'P95 (ms)':<9} | {'P99 (ms)':<9}"
    print(header)
    print("-" * 94)

    for name, s in stats.items():
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
    print(f"Report generation complete. Raw datasets available in: {os.path.abspath(args.out_dir)}\n")

if __name__ == "__main__":
    main()
