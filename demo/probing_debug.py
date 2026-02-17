import socket
import time
import os

print(f"PID: {os.getpid()}")
print("Normal operation...")
time.sleep(2)

print("Starting PROBING behavior (opening many sockets)...")
sockets = []
for i in range(60):
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sockets.append(s)
    except Exception as e:
        print(e)
print(f"Opened {len(sockets)} sockets.")
time.sleep(5)
