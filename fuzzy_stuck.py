import time
import random
import sys

print("Starting fuzzy loop process...")

counter = 0

while True:
    counter += 1
    # Introduce slight variation (random number at end)
    # The prefix is constant, so similarity should be high
    noise = random.randint(0, 5)
    
    # "Processing item 123 [noise: 3]"
    # "Processing item 124 [noise: 1]"
    # The Levenshtein distance between these should be small enough (< 10%)
    
    print(f"Processing data chunk {counter} [v:{noise}]", flush=True)
    
    # Use CPU
    st = time.time()
    while time.time() - st < 0.1:
        _ = 2**20
        
    time.sleep(0.05)
