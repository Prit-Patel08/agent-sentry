#!/usr/bin/env python3
"""Finite healthy worker with varied output and low CPU pressure."""

import random
import time


def main() -> None:
    print("healthy worker booted")
    for i in range(1, 19):
        queue = random.randint(1, 7)
        latency_ms = random.randint(45, 130)
        print(f"tick={i} queue_depth={queue} latency_ms={latency_ms} status=ok")
        time.sleep(0.35)
    print("healthy worker finished")


if __name__ == "__main__":
    main()
