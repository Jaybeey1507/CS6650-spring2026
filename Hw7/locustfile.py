from locust import HttpUser, task, between
import random

class OrderUser(HttpUser):
    wait_time = between(0.1, 0.5)

    @task
    def create_sync_order(self):
        payload = {
            "customer_id": random.randint(1, 1000),
            "items": [
                {
                    "product_id": random.randint(1, 100),
                    "quantity": random.randint(1, 3)
                }
            ]
        }
        self.client.post("/orders/sync", json=payload)