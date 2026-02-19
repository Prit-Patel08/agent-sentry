#!/usr/bin/env python3
import argparse
import os
import sys
import time


def main() -> int:
    parser = argparse.ArgumentParser(description="Slow memory leak fixture")
    parser.add_argument("--timeout", type=int, default=120, help="Self-terminate timeout in seconds")
    parser.add_argument("--chunk-mb", type=int, default=4, help="Megabytes to allocate per step")
    args = parser.parse_args()

    deadline = time.time() + max(1, args.timeout)
    blobs = []
    chunk_size = max(1, args.chunk_mb) * 1024 * 1024
    allocated_mb = 0

    while time.time() < deadline:
        blobs.append(bytearray(chunk_size))
        allocated_mb += args.chunk_mb
        print(f"leak-step pid={os.getpid()} allocated_mb={allocated_mb}", flush=True)
        time.sleep(0.2)

    print("timeout reached, exiting leak fixture", file=sys.stderr, flush=True)
    return 124


if __name__ == "__main__":
    raise SystemExit(main())
