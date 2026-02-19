#!/usr/bin/env python3
import argparse
import math
import random
import time


def heavy_step(size: int) -> float:
    values = [random.random() for _ in range(size)]
    # CPU-heavy but legitimate numerical workload.
    return sum(math.sin(v) * math.cos(v * 0.5) for v in values)


def main() -> int:
    parser = argparse.ArgumentParser(description="Healthy CPU spike fixture (false-positive guard)")
    parser.add_argument("--timeout", type=int, default=120, help="Self-terminate timeout in seconds")
    parser.add_argument("--spike-seconds", type=int, default=10, help="Duration of heavy compute burst")
    args = parser.parse_args()

    deadline = time.time() + max(1, args.timeout)
    spike_deadline = time.time() + max(1, args.spike_seconds)

    step = 0
    while time.time() < spike_deadline:
        step += 1
        value = heavy_step(50_000)
        print(f"progress step={step} phase=compute metric={value:.6f}", flush=True)
        if time.time() >= deadline:
            print("timeout reached during healthy spike", flush=True)
            return 124

    print("compute finished cleanly; flushing final state", flush=True)
    print("healthy workload completed", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
