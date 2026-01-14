import requests
import time
import matplotlib.pyplot as plt
import numpy as np

def load_test(url, duration_seconds=30):
    response_times = []
    start_time = time.time()
    end_time = start_time + duration_seconds

    print(f"Starting load test for {duration_seconds} seconds...")
    print(f"Target: {url}\n")

    while time.time() < end_time:
        try:
            t0 = time.time()
            r = requests.get(url, timeout=10)
            t1 = time.time()

            ms = (t1 - t0) * 1000
            response_times.append(ms)

            if r.status_code == 200:
                print(f"Request {len(response_times)}: {ms:.2f} ms")
            else:
                print(f"Request {len(response_times)}: status {r.status_code} in {ms:.2f} ms")

        except requests.exceptions.RequestException as e:
            print(f"Request failed: {e}")

    return response_times

EC2_URL = "http://35.88.23.190:8080/albums" 

times = load_test(EC2_URL, duration_seconds=30)

# Plot the results
plt.figure(figsize=(12, 8))

# Histogram
plt.subplot(2, 1, 1)
plt.hist(times, bins=50, alpha=0.7)
plt.xlabel("Response Time (ms)")
plt.ylabel("Frequency")
plt.title("Distribution of Response Times")

# Scatter plot over time
plt.subplot(2, 1, 2)
plt.scatter(range(len(times)), times, alpha=0.6)
plt.xlabel("Request Number")
plt.ylabel("Response Time (ms)")
plt.title("Response Times Over Time")

plt.tight_layout()
plt.show()

# Print statistics
print("\nStatistics:")
print(f"Total requests: {len(times)}")
print(f"Average response time: {np.mean(times):.2f} ms")
print(f"Median response time: {np.median(times):.2f} ms")
print(f"95th percentile: {np.percentile(times, 95):.2f} ms")
print(f"99th percentile: {np.percentile(times, 99):.2f} ms")
print(f"Max response time: {max(times):.2f} ms")
