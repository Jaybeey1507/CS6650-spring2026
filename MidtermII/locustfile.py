from locust import FastHttpUser, task, between

class SearchUser(FastHttpUser):
    wait_time = between(0.1, 0.3)

    @task(3)
    def normal_search(self):
        self.client.get("/products/search?q=book")

    @task(1)
    def chaos_search(self):
        self.client.get("/products/search?q=book&mode=chaos")