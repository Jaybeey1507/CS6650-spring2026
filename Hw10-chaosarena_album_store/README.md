# ChaosArena Album Store - Minimal Go Implementation

This is a fast, single-instance implementation meant to help you pass the critical ChaosArena scenarios quickly.

## Stack
- Go
- SQLite file database
- Local disk photo storage
- In-process background worker

## Files
- `main.go` - API server
- `go.mod` - dependencies
- `Dockerfile` - container build

## Endpoints
- `GET /health`
- `PUT /albums/{album_id}`
- `GET /albums/{album_id}`
- `GET /albums`
- `POST /albums/{album_id}/photos`
- `GET /albums/{album_id}/photos/{photo_id}`
- `DELETE /albums/{album_id}/photos/{photo_id}`
- `GET /files/{album_id}/{photo_id}`

## Local run
```bash
go mod download
go run main.go
```

## Local run with explicit base URL
```bash
PUBLIC_BASE_URL=http://YOUR_IP:8080 go run main.go
```

## Test commands
Replace the album ID with a real UUID.

```bash
curl http://localhost:8080/health
```

```bash
curl -X PUT http://localhost:8080/albums/a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  -H "Content-Type: application/json" \
  -d '{
    "album_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "title": "My Summer Trip",
    "description": "Photos from Cancun",
    "owner": "student@northeastern.edu"
  }'
```

```bash
curl http://localhost:8080/albums/a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

```bash
curl http://localhost:8080/albums
```

```bash
curl -X POST http://localhost:8080/albums/a1b2c3d4-e5f6-7890-abcd-ef1234567890/photos \
  -F "photo=@/path/to/image.jpg"
```

```bash
curl http://localhost:8080/albums/ALBUM_ID/photos/PHOTO_ID
```

```bash
curl -X DELETE http://localhost:8080/albums/ALBUM_ID/photos/PHOTO_ID
```

## Docker
```bash
docker build -t chaosarena-album-store .
docker run -p 8080:8080 -e PUBLIC_BASE_URL=http://YOUR_IP:8080 chaosarena-album-store
```

## Submission
Once deployed, submit your public URL:

```bash
curl -X POST https://chaosarena-alb-938452724.us-west-2.elb.amazonaws.com/submit \
  -H "Content-Type: application/json" \
  -d '{
    "email": "your@northeastern.edu",
    "nickname": "your-nickname",
    "base_url": "http://YOUR_PUBLIC_URL",
    "contract": "v1-album-store"
  }'
```
