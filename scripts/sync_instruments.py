#!/usr/bin/env python3
"""
Syncs instrument definitions (tokens, lot sizes, tick sizes, expiries)
from Zerodha Kite's public instruments API into the Postgres instruments table.
"""

import sys
import os
import csv
import urllib.request
import subprocess

EXCHANGES = ["MCX", "NFO"]

def sync_exchange(exchange, db_container="envoytrade-db"):
    url = f"https://api.kite.trade/instruments/{exchange}"
    print(f"Downloading instruments for {exchange} from {url}...")
    try:
        req = urllib.request.urlopen(url, timeout=30)
    except Exception as e:
        print(f"Failed to fetch {url}: {e}", file=sys.stderr)
        return False

    lines = [line.decode("utf-8") for line in req.readlines()]
    reader = csv.DictReader(lines)

    sql_statements = ["BEGIN;"]
    count = 0
    for r in reader:
        token = r.get("instrument_token")
        ex = r.get("exchange", exchange)
        sym = r.get("tradingsymbol", "").replace("'", "''")
        lot = r.get("lot_size", "1")
        tick = r.get("tick_size", "0.05")
        seg = r.get("segment", "")
        expiry = r.get("expiry")
        exp_sql = f"'{expiry}'" if expiry else "NULL"

        sql_statements.append(
            f"INSERT INTO instruments (instrument_token, exchange, tradingsymbol, lot_size, tick_size, segment, expiry) "
            f"VALUES ({token}, '{ex}', '{sym}', {lot}, {tick}, '{seg}', {exp_sql}) "
            f"ON CONFLICT (instrument_token) DO UPDATE SET "
            f"lot_size = EXCLUDED.lot_size, tradingsymbol = EXCLUDED.tradingsymbol, "
            f"exchange = EXCLUDED.exchange, tick_size = EXCLUDED.tick_size, "
            f"segment = EXCLUDED.segment, expiry = EXCLUDED.expiry, refreshed_at = now();"
        )
        count += 1

    sql_statements.append("COMMIT;")
    payload = "\n".join(sql_statements).encode("utf-8")

    cmd = ["podman", "exec", "-i", db_container, "psql", "-U", "envoytrade", "-d", "envoytrade", "-q"]
    proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    stdout, stderr = proc.communicate(input=payload)

    if proc.returncode != 0:
        print(f"Error inserting into DB: {stderr.decode()}", file=sys.stderr)
        return False

    print(f"Successfully synced {count} instruments for {exchange}.")
    return True

if __name__ == "__main__":
    exchanges = sys.argv[1:] if len(sys.argv) > 1 else EXCHANGES
    container = os.getenv("DB_CONTAINER", "envoytrade-db")
    for ex in exchanges:
        sync_exchange(ex, container)
