#!/usr/bin/env python3
import argparse
import math
import sys
import time


def main() -> int:
    parser = argparse.ArgumentParser(description="Runaway CPU + repetitive logs fixture")
    parser.add_argument("--timeout", type=int, default=120, help="Self-terminate timeout in seconds")
    args = parser.parse_args()

    deadline = time.time() + max(1, args.timeout)
    i = 0
    while time.time() < deadline:
        i += 1
        # Keep the output intentionally repetitive.
        print("processing request 4242 failed, retrying endlessly", flush=True)
        _ = math.sqrt((i % 1000) * 12345.6789)
    print("timeout reached, exiting runaway fixture", file=sys.stderr, flush=True)
    return 124


if __name__ == "__main__":
    raise SystemExit(main())
