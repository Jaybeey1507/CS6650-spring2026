from locust import FastHttpUser, task, between

class ProductSearchUser(FastHttpUser):
    wait_time = between(0.1, 0.2)

    @task
    def search_electronics(self):
        self.client.get("/products/search?q=Electronics")