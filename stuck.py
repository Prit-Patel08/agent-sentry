#!/usr/bin/env python3
"""Simple self-contained loop demo for Agent-Sentry."""

import time


def main() -> None:
    print("Starting stuck loop demo...")
    i = 0
    while True:
        i += 1
        print(f"[{i}] I am stuck in a loop")
        time.sleep(0.5)


if __name__ == "__main__":
    main()
