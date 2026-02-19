#!/usr/bin/env python3
import argparse
import subprocess
import sys


def main() -> int:
    parser = argparse.ArgumentParser(description="Spawn child and crash parent fixture")
    parser.add_argument("--timeout", type=int, default=120, help="Child self-terminate timeout in seconds")
    args = parser.parse_args()

    child_code = (
        "import time,sys\n"
        f"deadline=time.time()+{max(1, args.timeout)}\n"
        "i=0\n"
        "while time.time()<deadline:\n"
        "  i+=1\n"
        "  print(f'child-heartbeat {i}', flush=True)\n"
        "  time.sleep(0.5)\n"
        "print('child timeout reached', file=sys.stderr, flush=True)\n"
    )

    child = subprocess.Popen(["python3", "-c", child_code])
    print(f"spawned-child-pid={child.pid}", flush=True)

    # Crash/exit parent abruptly without cleaning up child.
    # Supervisor must still clean up via process group teardown.
    raise RuntimeError("intentional parent crash for zombie-spawner fixture")


if __name__ == "__main__":
    raise SystemExit(main())
