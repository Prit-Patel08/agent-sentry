import time
print("This script generates a lot of tokens to test cost tracking.")
for i in range(50):
    print(f"Generating token rich output line {i} with some random data to simulate an LLM response stream...")
    time.sleep(0.1)
