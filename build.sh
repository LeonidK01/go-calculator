#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p bin
gcc -std=c11 -shared -fPIC -O2 -Wall -Wextra -Werror -o bin/libcalculator.so native/c/calculator.c
cargo build --locked --release --manifest-path native/rust/Cargo.toml
cp native/rust/target/release/libcalculator_rust.so bin/
CGO_ENABLED=1 go build -trimpath -o bin/calculator_server ./cmd/calculator_server
CGO_ENABLED=0 go build -trimpath -o bin/generator ./cmd/generator
