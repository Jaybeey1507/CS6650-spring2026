from locust import FastHttpUser, task, between
import random

class ProductUser(FastHttpUser):
    wait_time = between(1, 2)

    @task(4)
    def get_product(self):
        self.client.get("/products/1")

    @task(1)
    def post_product_details(self):
        payload = {
            "product_id": 1,
            "sku": "ABC-123-XYZ",
            "manufacturer": "Acme Corporation",
            "category_id": 456,
            "weight": 1250,
            "some_other_id": 789
        }
        self.client.post("/products/1/details", json=payload)
