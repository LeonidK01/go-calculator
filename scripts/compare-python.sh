#!/usr/bin/env bash
# Run inside the test image with the original directory mounted read-only at /legacy.
set -euo pipefail
cd "$(dirname "$0")/.."
legacy="${1:-/legacy/calculator_server.py}"
command -v python3 >/dev/null
test -f "$legacy"
export GOMAXPROCS=4
pid=""
cleanup() {
    if [[ -n "$pid" ]]; then
        kill -INT "$pid" 2>/dev/null || true
        wait "$pid" || true
    fi
}
trap cleanup EXIT
for backend in python go; do
    if [[ "$backend" == python ]]; then
        python3 "$legacy" --host 127.0.0.1 --port 18080 \
            --c-lib "$PWD/bin/libcalculator.so" --rust-lib "$PWD/bin/libcalculator_rust.so" \
            --interval 3600 >/tmp/calculator-python.log 2>&1 &
    else
        ./bin/calculator_server --host 127.0.0.1 --port 18080 --concurrency 4 \
            --interval 3600 >/tmp/calculator-go.log 2>&1 &
    fi
    pid=$!
    ready=false
    for _ in {1..100}; do
        if (echo >/dev/tcp/127.0.0.1/18080) 2>/dev/null; then ready=true; break; fi
        sleep 0.05
    done
    if [[ "$ready" != true ]]; then cat "/tmp/calculator-$backend.log"; exit 1; fi
    for iteration in 1 2 3; do
        echo "backend=$backend iteration=$iteration GOMAXPROCS=4 workers=16 duration=5s"
        ./bin/generator --url http://127.0.0.1:18080/calc --threads 16 --interval 0 --duration 5
    done
    cleanup
    pid=""
done
