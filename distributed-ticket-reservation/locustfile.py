import os
import random
from locust import HttpUser, task, between

MODE = os.getenv("MODE", "baseline")

BASELINE_SEATS = [f"A{i}" for i in range(1, 101)]
CONTENTION_SEATS = ["A1", "A2", "A3", "A4", "A5"]


class TicketUser(HttpUser):
    wait_time = between(0.2, 1.0)

    def on_start(self):
        self.user_id = f"user-{random.randint(1000, 999999)}"

    @task(2)
    def list_seats(self):
        self.client.get("/events/evt-1/seats", name="GET /events/:id/seats")

    @task(3)
    def hold_and_confirm(self):
        if MODE == "contention":
            seat_id = random.choice(CONTENTION_SEATS)
        else:
            seat_id = random.choice(BASELINE_SEATS)

        hold_payload = {
            "event_id": "evt-1",
            "seat_id": seat_id,
            "user_id": self.user_id,
            "ttl_seconds": 30
        }

        with self.client.post(
            "/holds",
            json=hold_payload,
            name="POST /holds",
            catch_response=True
        ) as hold_resp:
            if hold_resp.status_code != 201:
                hold_resp.failure(f"hold failed: {hold_resp.text}")
                return

            try:
                hold_data = hold_resp.json()
                hold_id = hold_data["id"]
            except Exception:
                hold_resp.failure("invalid hold response")
                return

        confirm_payload = {
            "hold_id": hold_id,
            "user_id": self.user_id
        }

        with self.client.post(
            "/reservations/confirm",
            json=confirm_payload,
            name="POST /reservations/confirm",
            catch_response=True
        ) as confirm_resp:
            if confirm_resp.status_code != 201:
                confirm_resp.failure(f"confirm failed: {confirm_resp.text}")
            else:
                confirm_resp.success()