#!/usr/bin/env python3
"""Loop demo with small line variations to exercise fuzzy detection."""

import random
import time


def main() -> None:
    print("Starting fuzzy loop demo...")
    nouns = ["token", "request", "item", "batch"]
    while True:
        noun = random.choice(nouns)
        value = random.randint(100, 999)
        print(f"Processing {noun} {value} at 0x{value:08x} with value {value / 10:.1f}")
        time.sleep(0.35)


if __name__ == "__main__":
    main()
