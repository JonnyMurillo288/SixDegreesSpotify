# This is going to be a python file to run a continuous loop of random artists
# The main point of this is to fill up my database with as much data as I can to create some tests for the algorithm
import subprocess
import random
import time
from datetime import datetime

artist_pairs = [
    ("Taylor Swift", "Metallica"),
    ("Drake", "Bon Iver"),
    ("Bad Bunny", "Foo Fighters"),
    ("Kendrick Lamar", "Radiohead"),
    ("Beyoncé", "Daft Punk"),
]

def run_pair(start, target):
    cmd = ["go", "run", "main.go", "-start", start, "-find", target]
    start_time = datetime.now()
    print(f"\n[{start_time:%Y-%m-%d %H:%M:%S}] ▶ Running: {' '.join(cmd)}")

    # Stream stdout/stderr live
    process = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    for line in process.stdout:
        print(line, end="")  # stream each line as it arrives
    process.wait()

    end_time = datetime.now()
    duration = (end_time - start_time).total_seconds()

    if process.returncode == 0:
        print(f"[{end_time:%H:%M:%S}] ✅ Completed {start} → {target} in {duration:.1f}s")
    else:
        print(f"[{end_time:%H:%M:%S}] ❌ Failed {start} → {target} (code {process.returncode}) in {duration:.1f}s")

    return process.returncode, duration


def main():
    print("Starting continuous verbose run of artist combinations...\n")
    while True:
        start, target = random.choice(artist_pairs)
        run_pair(start, target)
        time.sleep(3)  # brief pause between runs


if __name__ == "__main__":
    main()