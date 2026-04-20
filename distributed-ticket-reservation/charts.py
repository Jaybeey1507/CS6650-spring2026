import matplotlib.pyplot as plt


def chart_concurrent_hold_attempts():
    labels = ["Successful hold", "Failed hold"]
    values = [1, 19]

    plt.figure(figsize=(8, 5))
    plt.bar(labels, values)
    plt.title("Concurrent Hold Attempts on the Same Seat")
    plt.xlabel("Outcome")
    plt.ylabel("Count")
    plt.tight_layout()
    plt.savefig("chart_concurrent_hold_attempts.png", dpi=300)
    plt.show()


def chart_seat_status_transition():
    labels = ["Available", "Reserved"]
    before = [5, 0]
    after = [4, 1]

    x = range(len(labels))
    width = 0.35

    plt.figure(figsize=(8, 5))
    plt.bar([i - width / 2 for i in x], before, width=width, label="Before reservation")
    plt.bar([i + width / 2 for i in x], after, width=width, label="After reservation")

    plt.xticks(list(x), labels)
    plt.title("Seat Status Before and After Reservation")
    plt.xlabel("Seat Status")
    plt.ylabel("Number of Seats")
    plt.legend()
    plt.tight_layout()
    plt.savefig("chart_seat_status_transition.png", dpi=300)
    plt.show()


if __name__ == "__main__":
    chart_concurrent_hold_attempts()
    chart_seat_status_transition()
    print("Charts created:")
    print(" - chart_concurrent_hold_attempts.png")
    print(" - chart_seat_status_transition.png")