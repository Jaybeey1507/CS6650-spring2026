package main

import (
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "mime/multipart"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"
)

type App struct {
    store         *Store
    jobs          chan string
    storageDir    string
    publicBaseURL string
}

type Store struct {
    mu       sync.RWMutex
    dataDir  string
    albums   map[string]*AlbumRecord
    photos   map[string]*PhotoRecord
}

type Album struct {
    AlbumID     string `json:"album_id"`
    Title       string `json:"title"`
    Description string `json:"description"`
    Owner       string `json:"owner"`
}

type AlbumRequest struct {
    AlbumID     string `json:"album_id"`
    Title       string `json:"title"`
    Description string `json:"description"`
    Owner       string `json:"owner"`
}

type AlbumRecord struct {
    Album
    NextSeq   int64     `json:"next_seq"`
    CreatedAt time.Time `json:"created_at"`
}

type PhotoAccepted struct {
    PhotoID string `json:"photo_id"`
    Seq     int64  `json:"seq"`
    Status  string `json:"status"`
}

type PhotoStatus struct {
    PhotoID string `json:"photo_id"`
    AlbumID string `json:"album_id"`
    Seq     int64  `json:"seq"`
    Status  string `json:"status"`
    URL     string `json:"url,omitempty"`
}

type PhotoRecord struct {
    PhotoID    string    `json:"photo_id"`
    AlbumID    string    `json:"album_id"`
    Seq        int64     `json:"seq"`
    Status     string    `json:"status"`
    TempPath   string    `json:"temp_path"`
    FilePath   string    `json:"file_path"`
    Deleted    bool      `json:"deleted"`
    CreatedAt  time.Time `json:"created_at"`
}

type ErrorResponse struct {
    Error string `json:"error"`
}

func main() {
    port := getenv("PORT", "8080")
    dataDir := getenv("DATA_DIR", "./data")
    storageDir := getenv("STORAGE_DIR", "./storage")
    publicBaseURL := strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/")

    if err := os.MkdirAll(dataDir, 0o755); err != nil {
        log.Fatalf("create data dir: %v", err)
    }
    if err := os.MkdirAll(filepath.Join(storageDir, "tmp"), 0o755); err != nil {
        log.Fatalf("create tmp dir: %v", err)
    }
    if err := os.MkdirAll(filepath.Join(storageDir, "albums"), 0o755); err != nil {
        log.Fatalf("create albums dir: %v", err)
    }

    store, err := NewStore(dataDir)
    if err != nil {
        log.Fatalf("init store: %v", err)
    }

    app := &App{
        store:         store,
        jobs:          make(chan string, 1024),
        storageDir:    storageDir,
        publicBaseURL: publicBaseURL,
    }

    for i := 0; i < 4; i++ {
        go app.worker(i + 1)
    }

    mux := http.NewServeMux()
    mux.HandleFunc("/health", app.handleHealth)
    mux.HandleFunc("/albums", app.handleAlbumsRoot)
    mux.HandleFunc("/albums/", app.handleAlbumsSubroutes)
    mux.HandleFunc("/files/", app.handleFiles)

    srv := &http.Server{
        Addr:              ":" + port,
        Handler:           loggingMiddleware(mux),
        ReadHeaderTimeout: 10 * time.Second,
        ReadTimeout:       5 * time.Minute,
        WriteTimeout:      5 * time.Minute,
        IdleTimeout:       60 * time.Second,
    }

    log.Printf("listening on :%s", port)
    log.Printf("data=%s storage=%s", dataDir, storageDir)
    if publicBaseURL != "" {
        log.Printf("public base url=%s", publicBaseURL)
    }

    log.Fatal(srv.ListenAndServe())
}

func NewStore(dataDir string) (*Store, error) {
    s := &Store{
        dataDir: dataDir,
        albums:  map[string]*AlbumRecord{},
        photos:  map[string]*PhotoRecord{},
    }
    if err := s.load(); err != nil {
        return nil, err
    }
    return s, nil
}

func (s *Store) load() error {
    if err := loadJSON(filepath.Join(s.dataDir, "albums.json"), &s.albums); err != nil {
        return err
    }
    if err := loadJSON(filepath.Join(s.dataDir, "photos.json"), &s.photos); err != nil {
        return err
    }
    if s.albums == nil {
        s.albums = map[string]*AlbumRecord{}
    }
    if s.photos == nil {
        s.photos = map[string]*PhotoRecord{}
    }
    return nil
}

func (s *Store) persistLocked() error {
    if err := writeJSONFile(filepath.Join(s.dataDir, "albums.json"), s.albums); err != nil {
        return err
    }
    if err := writeJSONFile(filepath.Join(s.dataDir, "photos.json"), s.photos); err != nil {
        return err
    }
    return nil
}

func (s *Store) UpsertAlbum(req AlbumRequest) (Album, bool, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    existing, ok := s.albums[req.AlbumID]
    created := !ok
    if !ok {
        existing = &AlbumRecord{
            Album: Album{
                AlbumID:     req.AlbumID,
                Title:       req.Title,
                Description: req.Description,
                Owner:       req.Owner,
            },
            NextSeq:   1,
            CreatedAt: time.Now().UTC(),
        }
        s.albums[req.AlbumID] = existing
    } else {
        existing.Title = req.Title
        existing.Description = req.Description
        existing.Owner = req.Owner
    }

    if err := s.persistLocked(); err != nil {
        return Album{}, false, err
    }
    return existing.Album, created, nil
}

func (s *Store) GetAlbum(albumID string) (Album, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    rec, ok := s.albums[albumID]
    if !ok {
        return Album{}, false
    }
    return rec.Album, true
}

func (s *Store) ListAlbums() []Album {
    s.mu.RLock()
    defer s.mu.RUnlock()

    out := make([]Album, 0, len(s.albums))
    for _, rec := range s.albums {
        out = append(out, rec.Album)
    }
    return out
}

func (s *Store) CreatePhoto(albumID, tempPath string) (PhotoAccepted, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    album, ok := s.albums[albumID]
    if !ok {
        return PhotoAccepted{}, os.ErrNotExist
    }

    photoID := newUUID()
    seq := album.NextSeq
    album.NextSeq++

    s.photos[photoKey(albumID, photoID)] = &PhotoRecord{
        PhotoID:   photoID,
        AlbumID:   albumID,
        Seq:       seq,
        Status:    "processing",
        TempPath:  tempPath,
        FilePath:  "",
        Deleted:   false,
        CreatedAt: time.Now().UTC(),
    }

    if err := s.persistLocked(); err != nil {
        return PhotoAccepted{}, err
    }

    return PhotoAccepted{PhotoID: photoID, Seq: seq, Status: "processing"}, nil
}

func (s *Store) GetPhoto(albumID, photoID string) (PhotoRecord, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    rec, ok := s.photos[photoKey(albumID, photoID)]
    if !ok || rec.Deleted {
        return PhotoRecord{}, false
    }
    return *rec, true
}

func (s *Store) MarkDeleted(albumID, photoID string) (PhotoRecord, bool, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    rec, ok := s.photos[photoKey(albumID, photoID)]
    if !ok {
        return PhotoRecord{}, false, nil
    }
    rec.Deleted = true
    if err := s.persistLocked(); err != nil {
        return PhotoRecord{}, false, err
    }
    return *rec, true, nil
}

func (s *Store) RemovePhoto(albumID, photoID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    delete(s.photos, photoKey(albumID, photoID))
    return s.persistLocked()
}

func (s *Store) CompletePhoto(albumID, photoID, finalPath string) bool {
    s.mu.Lock()
    defer s.mu.Unlock()

    rec, ok := s.photos[photoKey(albumID, photoID)]
    if !ok || rec.Deleted {
        return false
    }
    rec.Status = "completed"
    rec.FilePath = finalPath
    rec.TempPath = ""
    _ = s.persistLocked()
    return true
}

func (s *Store) MarkFailed(albumID, photoID string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    rec, ok := s.photos[photoKey(albumID, photoID)]
    if !ok || rec.Deleted {
        return
    }
    rec.Status = "failed"
    _ = s.persistLocked()
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleAlbumsRoot(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
        return
    }
    writeJSON(w, http.StatusOK, a.store.ListAlbums())
}

func (a *App) handleAlbumsSubroutes(w http.ResponseWriter, r *http.Request) {
    parts := splitPath(strings.TrimPrefix(r.URL.Path, "/albums/"))

    if len(parts) == 1 {
        albumID := parts[0]
        switch r.Method {
        case http.MethodPut:
            a.handlePutAlbum(w, r, albumID)
        case http.MethodGet:
            a.handleGetAlbum(w, r, albumID)
        default:
            writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
        }
        return
    }

    if len(parts) == 2 && parts[1] == "photos" {
        if r.Method == http.MethodPost {
            a.handleUploadPhoto(w, r, parts[0])
            return
        }
        writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
        return
    }

    if len(parts) == 3 && parts[1] == "photos" {
        switch r.Method {
        case http.MethodGet:
            a.handleGetPhoto(w, r, parts[0], parts[2])
        case http.MethodDelete:
            a.handleDeletePhoto(w, r, parts[0], parts[2])
        default:
            writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
        }
        return
    }

    writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
}

func (a *App) handlePutAlbum(w http.ResponseWriter, r *http.Request, albumID string) {
    if !isUUID(albumID) {
        writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid album_id"})
        return
    }

    var req AlbumRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid json"})
        return
    }
    if req.AlbumID == "" || req.Title == "" || req.Description == "" || req.Owner == "" {
        writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "missing required fields"})
        return
    }
    if req.AlbumID != albumID {
        writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "album_id mismatch"})
        return
    }

    album, created, err := a.store.UpsertAlbum(req)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
        return
    }
    status := http.StatusOK
    if created {
        status = http.StatusCreated
    }
    writeJSON(w, status, album)
}

func (a *App) handleGetAlbum(w http.ResponseWriter, r *http.Request, albumID string) {
    album, ok := a.store.GetAlbum(albumID)
    if !ok {
        writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
        return
    }
    writeJSON(w, http.StatusOK, album)
}

func (a *App) handleUploadPhoto(w http.ResponseWriter, r *http.Request, albumID string) {
    if !isUUID(albumID) {
        writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid album_id"})
        return
    }
    if _, ok := a.store.GetAlbum(albumID); !ok {
        writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
        return
    }

    const maxUploadSize = 210 << 20
    r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

    if err := r.ParseMultipartForm(32 << 20); err != nil {
        writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "missing or malformed photo field"})
        return
    }

    file, _, err := r.FormFile("photo")
    if err != nil {
        writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "missing or malformed photo field"})
        return
    }
    defer file.Close()

    tempPath := filepath.Join(a.storageDir, "tmp", newUUID()+".upload")
    if err := copyToFile(file, tempPath); err != nil {
        writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
        return
    }

    accepted, err := a.store.CreatePhoto(albumID, tempPath)
    if err != nil {
        _ = os.Remove(tempPath)
        if os.IsNotExist(err) {
            writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
            return
        }
        writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
        return
    }

    select {
    case a.jobs <- photoKey(albumID, accepted.PhotoID):
    default:
        go func() { a.jobs <- photoKey(albumID, accepted.PhotoID) }()
    }

    writeJSON(w, http.StatusAccepted, accepted)
}

func (a *App) handleGetPhoto(w http.ResponseWriter, r *http.Request, albumID, photoID string) {
    rec, ok := a.store.GetPhoto(albumID, photoID)
    if !ok {
        writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
        return
    }

    resp := PhotoStatus{
        PhotoID: rec.PhotoID,
        AlbumID: rec.AlbumID,
        Seq:     rec.Seq,
        Status:  rec.Status,
    }
    if rec.Status == "completed" && rec.FilePath != "" {
        resp.URL = a.fileURL(r, albumID, photoID)
    }
    writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleDeletePhoto(w http.ResponseWriter, r *http.Request, albumID, photoID string) {
    rec, found, err := a.store.MarkDeleted(albumID, photoID)
    if err != nil {
        w.WriteHeader(http.StatusNoContent)
        return
    }
    if !found {
        w.WriteHeader(http.StatusNoContent)
        return
    }

    _ = os.Remove(rec.TempPath)
    _ = os.Remove(rec.FilePath)
    _ = a.store.RemovePhoto(albumID, photoID)
    w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleFiles(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
        return
    }
    parts := splitPath(strings.TrimPrefix(r.URL.Path, "/files/"))
    if len(parts) != 2 {
        writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
        return
    }

    rec, ok := a.store.GetPhoto(parts[0], parts[1])
    if !ok || rec.Status != "completed" || rec.FilePath == "" {
        writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
        return
    }
    if _, err := os.Stat(rec.FilePath); err != nil {
        writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
        return
    }
    http.ServeFile(w, r, rec.FilePath)
}

func (a *App) worker(id int) {
    for key := range a.jobs {
        if err := a.processPhoto(key); err != nil {
            log.Printf("worker=%d key=%s err=%v", id, key, err)
        }
    }
}

func (a *App) processPhoto(key string) error {
    albumID, photoID, ok := parsePhotoKey(key)
    if !ok {
        return fmt.Errorf("bad key")
    }

    rec, found := a.store.GetPhoto(albumID, photoID)
    if !found {
        return nil
    }
    if rec.Deleted {
        _ = os.Remove(rec.TempPath)
        return nil
    }

    finalDir := filepath.Join(a.storageDir, "albums", albumID)
    if err := os.MkdirAll(finalDir, 0o755); err != nil {
        a.store.MarkFailed(albumID, photoID)
        return err
    }
    finalPath := filepath.Join(finalDir, photoID+".bin")

    if err := os.Rename(rec.TempPath, finalPath); err != nil {
        a.store.MarkFailed(albumID, photoID)
        return err
    }

    if !a.store.CompletePhoto(albumID, photoID, finalPath) {
        _ = os.Remove(finalPath)
    }
    return nil
}

func (a *App) fileURL(r *http.Request, albumID, photoID string) string {
    base := a.publicBaseURL
    if base == "" {
        base = requestBaseURL(r)
    }
    return fmt.Sprintf("%s/files/%s/%s", base, albumID, photoID)
}

func requestBaseURL(r *http.Request) string {
    if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
        return proto + "://" + r.Host
    }
    if r.TLS != nil {
        return "https://" + r.Host
    }
    return "http://" + r.Host
}

func photoKey(albumID, photoID string) string {
    return albumID + ":" + photoID
}

func parsePhotoKey(k string) (string, string, bool) {
    parts := strings.SplitN(k, ":", 2)
    if len(parts) != 2 {
        return "", "", false
    }
    return parts[0], parts[1], true
}

func splitPath(p string) []string {
    p = strings.Trim(p, "/")
    if p == "" {
        return nil
    }
    return strings.Split(p, "/")
}

func isUUID(s string) bool {
    if len(s) != 36 {
        return false
    }
    for i, c := range s {
        switch i {
        case 8, 13, 18, 23:
            if c != '-' {
                return false
            }
        default:
            if !isHexByte(byte(c)) {
                return false
            }
        }
    }
    return true
}

func isHexByte(b byte) bool {
    return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func newUUID() string {
    var b [16]byte
    _, _ = rand.Read(b[:])
    b[6] = (b[6] & 0x0f) | 0x40
    b[8] = (b[8] & 0x3f) | 0x80
    hexStr := hex.EncodeToString(b[:])
    return fmt.Sprintf("%s-%s-%s-%s-%s", hexStr[0:8], hexStr[8:12], hexStr[12:16], hexStr[16:20], hexStr[20:32])
}

func copyToFile(src multipart.File, path string) error {
    dst, err := os.Create(path)
    if err != nil {
        return err
    }
    defer dst.Close()

    if _, err := io.Copy(dst, src); err != nil {
        return err
    }
    return dst.Sync()
}

func loadJSON(path string, v any) error {
    f, err := os.Open(path)
    if os.IsNotExist(err) {
        return nil
    }
    if err != nil {
        return err
    }
    defer f.Close()
    return json.NewDecoder(f).Decode(v)
}

func writeJSONFile(path string, v any) error {
    tmp := path + ".tmp"
    f, err := os.Create(tmp)
    if err != nil {
        return err
    }
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    if err := enc.Encode(v); err != nil {
        _ = f.Close()
        return err
    }
    if err := f.Close(); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    if v != nil {
        _ = json.NewEncoder(w).Encode(v)
    }
}

func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
    })
}

func getenv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
