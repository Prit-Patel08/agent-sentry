import time
import random
import sys

# Consume CPU
def burn_cpu(duration):
    start = time.time()
    while time.time() - start < duration:
        pass

print("Starting stuck process...")
i = 0
while True:
    i += 1
    # Print a log that changes every time but has same structure
    # Timestamp, Hex ID, Random Number
    print(f"[{time.time()}] Processing item {i} at 0x{id(i):x} with value {random.random()}")
    sys.stdout.flush()
    
    # Burn CPU to trigger the threshold (default 90% is high, let's try to hit it)
    # We burn for 0.1s, then print. This might not reach 90% average if print is fast.
    # Let's burn for longer relative to print.
    burn_cpu(0.05)
