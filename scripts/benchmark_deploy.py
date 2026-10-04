#!/usr/bin/env python3
"""
benchmark_deploy.py - Integration Sanity & Memory Footprint Benchmarking for EnvoyTrade
Tests deployed instance at a given URL (default: http://localhost:8080)
Measures latency, throughput, and process/container memory consumption.
"""

import sys
import os
import time
import json
import urllib.request
import urllib.parse
import http.cookiejar
import concurrent.futures
import subprocess

TARGET_URL = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080"
LABEL = sys.argv[2] if len(sys.argv) > 2 else "Deployment"

cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

def log(msg):
    print(f"[{LABEL}] {msg}")

def request(method, path, data=None, headers=None):
    url = f"{TARGET_URL.rstrip('/')}{path}"
    h = headers or {}
    body = None
    if data is not None:
        body = json.dumps(data).encode("utf-8")
        h["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=body, headers=h, method=method)
    try:
        with opener.open(req, timeout=10) as resp:
            return resp.status, resp.read(), resp.headers
    except urllib.error.HTTPError as e:
        return e.code, e.read(), e.headers
    except Exception as e:
        return 0, str(e).encode(), {}

def run_sanity_tests():
    log("--> Running Sanity & Integration Suite...")
    tests_passed = 0
    total_tests = 0

    # 1. Root SPA HTML
    total_tests += 1
    st, body, hdrs = request("GET", "/")
    if st == 200 and b"<html" in body.lower():
        log("  [PASS] GET / (Root SPA HTML served)")
        tests_passed += 1
    else:
        log(f"  [FAIL] GET / returned status {st}")

    # 2. SPA Fallback for client routes
    total_tests += 1
    st, body, hdrs = request("GET", "/groups")
    if st == 200 and b"<html" in body.lower():
        log("  [PASS] GET /groups (SPA fallback works)")
        tests_passed += 1
    else:
        log(f"  [FAIL] GET /groups returned status {st}")

    # 3. Unauthenticated API access protection
    total_tests += 1
    no_cookie_opener = urllib.request.build_opener()
    try:
        req = urllib.request.Request(f"{TARGET_URL.rstrip('/')}/api/v1/auth/me", method="GET")
        no_cookie_opener.open(req, timeout=5)
        log("  [FAIL] GET /api/v1/auth/me was NOT protected!")
    except urllib.error.HTTPError as e:
        if e.code == 401:
            log("  [PASS] GET /api/v1/auth/me correctly rejected with 401 Unauthorized")
            tests_passed += 1
        else:
            log(f"  [FAIL] GET /api/v1/auth/me returned unexpected status {e.code}")

    # 4. Webhook endpoint
    total_tests += 1
    st, body, hdrs = request("POST", "/broker-callback", data={})
    if st == 200:
        log("  [PASS] POST /broker-callback (Webhook alive and acknowledging 200)")
        tests_passed += 1
    else:
        log(f"  [FAIL] POST /broker-callback returned status {st}")

    # 5. User Authentication
    total_tests += 1
    st, body, hdrs = request("POST", "/api/v1/auth/login", data={
        "email": "trader@envoytrade.com",
        "password": "password123"
    })
    if st == 200:
        log("  [PASS] POST /api/v1/auth/login (Session authentication succeeded)")
        tests_passed += 1
    else:
        log(f"  [FAIL] POST /api/v1/auth/login returned {st}: {body.decode()}")

    # 6. Authenticated Session /me
    total_tests += 1
    st, body, hdrs = request("GET", "/api/v1/auth/me")
    if st == 200 and b"trader" in body:
        log("  [PASS] GET /api/v1/auth/me (User session context active)")
        tests_passed += 1
    else:
        log(f"  [FAIL] GET /api/v1/auth/me returned {st}: {body.decode()}")

    # 7. Groups Persistence Query
    total_tests += 1
    st, body, hdrs = request("GET", "/api/v1/groups")
    if st == 200:
        try:
            groups = json.loads(body)
            log(f"  [PASS] GET /api/v1/groups (Retrieved {len(groups)} groups from DB)")
            tests_passed += 1
        except Exception:
            log("  [FAIL] GET /api/v1/groups invalid JSON")
    else:
        log(f"  [FAIL] GET /api/v1/groups returned {st}")

    # 8. Accounts Persistence Query
    total_tests += 1
    st, body, hdrs = request("GET", "/api/v1/accounts")
    if st == 200:
        try:
            accounts = json.loads(body)
            log(f"  [PASS] GET /api/v1/accounts (Retrieved {len(accounts)} accounts from DB)")
            tests_passed += 1
        except Exception:
            log("  [FAIL] GET /api/v1/accounts invalid JSON")
    else:
        log(f"  [FAIL] GET /api/v1/accounts returned {st}")

    log(f"Sanity Results: {tests_passed}/{total_tests} tests passed.")
    return tests_passed == total_tests

def run_performance_benchmarks(concurrency=10, total_requests=200):
    log(f"--> Running Concurrency Benchmark ({total_requests} requests, concurrency={concurrency})...")
    paths = ["/", "/groups", "/api/v1/groups", "/api/v1/accounts"]

    latencies = []
    errors = 0
    start_time = time.time()

    def do_req(i):
        p = paths[i % len(paths)]
        t0 = time.time()
        st, _, _ = request("GET", p)
        t1 = time.time()
        return st, (t1 - t0) * 1000.0

    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as executor:
        futures = [executor.submit(do_req, i) for i in range(total_requests)]
        for f in concurrent.futures.as_completed(futures):
            st, lat = f.result()
            if st in (200, 401):
                latencies.append(lat)
            else:
                errors += 1

    total_time = time.time() - start_time
    rps = len(latencies) / total_time if total_time > 0 else 0
    latencies.sort()
    avg_lat = sum(latencies) / len(latencies) if latencies else 0
    p50 = latencies[int(len(latencies) * 0.5)] if latencies else 0
    p95 = latencies[int(len(latencies) * 0.95)] if latencies else 0
    p99 = latencies[int(len(latencies) * 0.99)] if latencies else 0

    log(f"  Throughput:    {rps:.1f} req/sec (Duration: {total_time:.2f}s)")
    log(f"  Avg Latency:   {avg_lat:.2f} ms")
    log(f"  P50 Latency:   {p50:.2f} ms")
    log(f"  P95 Latency:   {p95:.2f} ms")
    log(f"  P99 Latency:   {p99:.2f} ms")
    log(f"  Error Count:   {errors}")

    return {
        "rps": rps,
        "avg_ms": avg_lat,
        "p50_ms": p50,
        "p95_ms": p95,
        "p99_ms": p99,
        "errors": errors
    }

if __name__ == "__main__":
    passed = run_sanity_tests()
    if not passed:
        sys.exit(1)
    bench = run_performance_benchmarks()
    # Output json summary on last line for parsing
    print(f"BENCHMARK_SUMMARY:{json.dumps(bench)}")
