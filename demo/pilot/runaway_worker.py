#!/usr/bin/env python3
"""Intentional runaway loop with repetitive output and sustained CPU load."""

import math


def main() -> None:
    print("runaway worker booted")
    i = 0
    while True:
        for j in range(120000):
            _ = math.sqrt((j % 31) + 1)
        if i % 2 == 0:
            print("processing request 4242 failed, retrying endlessly", flush=True)
        i += 1


if __name__ == "__main__":
    main()
