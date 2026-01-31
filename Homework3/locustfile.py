from locust import FastHttpUser, task, between
import random

class AlbumUser(FastHttpUser):
    wait_time = between(1, 2)

    @task(3)
    def get_albums(self):
        self.client.get("/albums")

    @task(1)
    def post_album(self):
        album_id = str(random.randint(100000, 999999))
        payload = {
            "id": album_id,
            "title": "LoadTest Album",
            "artist": "Locust",
            "price": 9.99
        }
        self.client.post("/albums", json=payload)
