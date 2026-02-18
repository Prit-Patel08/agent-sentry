#!/usr/bin/env python3
"""Worker with short bursts that should remain recoverable/non-runaway."""

import math
import random
import time


def brief_cpu_burst(iterations: int) -> None:
    acc = 0.0
    for i in range(iterations):
        acc += math.sqrt((i % 97) + 1)
    if acc < 0:
        print(acc)


def main() -> None:
    print("bursty worker booted")
    for i in range(1, 15):
        if i % 4 == 0:
            brief_cpu_burst(250000)
            phase = "burst"
        else:
            phase = "steady"
            time.sleep(0.25)
        jobs = random.randint(2, 11)
        print(f"tick={i} phase={phase} jobs={jobs} status=ok")
    print("bursty worker finished")


if __name__ == "__main__":
    main()
