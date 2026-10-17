// HomeLite — a lightweight, self-hosted cloud storage server.
//
// Build: go build -o homelite.exe .
// Run:   .\homelite.exe
//
// The dashboard UI lives in index.html and is embedded into the binary at
// compile time via //go:embed. The resulting .exe is fully standalone.

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/argon2"
	_ "modernc.org/sqlite"
)

//go:embed index.html
var assets embed.FS

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	defaultPort         = 8080
	defaultHost         = "0.0.0.0"
	defaultMaxUpload    = int64(5 * 1024 * 1024 * 1024)  // 5 GiB
	defaultQuota        = int64(50 * 1024 * 1024 * 1024) // 50 GiB
	defaultSessionHours = int64(72)
	sessionCookieName   = "hl_session"
	csrfHeader          = "X-CSRF-Token"
	maxConcurrentUpload = 4
	loginMaxAttempts    = 8
	loginWindow         = 15 * time.Minute
)

// ---------------------------------------------------------------------------
// ANSI splash
// ---------------------------------------------------------------------------

const (
	ansiReset     = "\x1b[0m"
	ansiBold      = "\x1b[1m"
	ansiDim       = "\x1b[2m"
	ansiClearLine = "\x1b[2K\r"
	ansiCyan      = "\x1b[38;5;45m"
	ansiBlue      = "\x1b[38;5;39m"
	ansiMagenta   = "\x1b[38;5;170m"
	ansiGreen     = "\x1b[38;5;42m"
	ansiYellow    = "\x1b[38;5;220m"
	ansiGrey      = "\x1b[38;5;245m"
	ansiWhite     = "\x1b[38;5;255m"
)

func playSplash() {
	enableVT()
	time.Sleep(120 * time.Millisecond)

	lines := []string{
		`  ██╗  ██╗ ██████╗ ███╗   ███╗███████╗██╗     ██╗████████╗███████╗`,
		`  ██║  ██║██╔═══██╗████╗ ████║██╔════╝██║     ██║╚══██╔══╝██╔════╝`,
		`  ███████║██║   ██║██╔████╔██║█████╗  ██║     ██║   ██║   █████╗  `,
		`  ██╔══██║██║   ██║██║╚██╔╝██║██╔══╝  ██║     ██║   ██║   ██╔══╝  `,
		`  ██║  ██║╚██████╔╝██║ ╚═╝ ██║███████╗███████╗██║   ██║   ███████╗`,
		`  ╚═╝  ╚═╝ ╚═════╝ ╚═╝     ╚═╝╚══════╝╚══════╝╚═╝   ╚═╝   ╚══════╝`,
	}
	colors := []string{ansiCyan, ansiBlue, ansiMagenta, ansiMagenta, ansiBlue, ansiCyan}
	for i, line := range lines {
		fmt.Printf("%s%s%s%s\n", colors[i], ansiBold, line, ansiReset)
		time.Sleep(35 * time.Millisecond)
	}
	fmt.Println()

	tagline := "  · private cloud · one binary · zero cloud bills ·"
	fmt.Printf("%s%s%s\n\n", ansiGrey, tagline, ansiReset)

	spinnerLine("initializing modules", 700*time.Millisecond)
	spinnerLine("preparing storage engine", 700*time.Millisecond)
	progressBarLine("booting HomeLite core", 900*time.Millisecond, 36)
	fmt.Println()
}

func spinnerLine(label string, total time.Duration) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	steps := 30
	step := total / time.Duration(steps)
	for i := 0; i < steps; i++ {
		fmt.Printf("%s%s%s %s%s%s", ansiClearLine, ansiYellow,
			frames[i%len(frames)], ansiWhite, label, ansiReset)
		time.Sleep(step)
	}
	fmt.Printf("%s%s✓%s %s%s\n", ansiClearLine, ansiGreen, ansiReset, label, ansiReset)
}

func progressBarLine(label string, total time.Duration, width int) {
	steps := 40
	step := total / time.Duration(steps)
	for i := 0; i <= steps; i++ {
		filled := (i * width) / steps
		empty := width - filled
		fmt.Printf("%s  %s%s%s%s%s%s  %s%s%s",
			ansiClearLine, ansiCyan,
			strings.Repeat("█", filled),
			ansiDim, strings.Repeat("░", empty), ansiReset,
			ansiGrey, label, ansiReset)
		time.Sleep(step)
	}
	fmt.Println()
}

// ---------------------------------------------------------------------------
// App state
// ---------------------------------------------------------------------------

type AppState struct {
	db             *sql.DB
	root           string
	filesDir       string
	tempDir        string
	uploadSem      chan struct{}
	loginAttempts  sync.Map
	bytesServed    atomic.Int64
	startedAt      time.Time
	maxUploadBytes atomic.Int64
}

type attemptEntry struct {
	mu    sync.Mutex
	count int
	first time.Time
}

func (s *AppState) filesDirFor(userID int64) string {
	return filepath.Join(s.filesDir, strconv.FormatInt(userID, 10))
}

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

type User struct {
	ID         int64     `json:"id"`
	Username   string    `json:"username"`
	Role       string    `json:"role"`
	QuotaBytes int64     `json:"quota_bytes"`
	UsedBytes  int64     `json:"used_bytes"`
	Disabled   bool      `json:"disabled"`
	CreatedAt  time.Time `json:"created_at"`
}

type FileEntry struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id"`
	Name      string    `json:"name"`
	IsDir     bool      `json:"is_dir"`
	Size      int64     `json:"size"`
	Mime      string    `json:"mime"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

type AppErr struct {
	Status int
	Code   string
	Msg    string
}

func (e *AppErr) Error() string { return e.Msg }

func errUnauthorized() *AppErr       { return &AppErr{401, "unauthorized", "Authentication required"} }
func errForbidden() *AppErr          { return &AppErr{403, "forbidden", "Access denied"} }
func errNotFound() *AppErr           { return &AppErr{404, "not_found", "Resource not found"} }
func errBadRequest(m string) *AppErr { return &AppErr{400, "bad_request", m} }
func errConflict(m string) *AppErr   { return &AppErr{409, "conflict", m} }
func errTooMany() *AppErr            { return &AppErr{429, "too_many_requests", "Too many attempts"} }
func errPayloadTooLarge() *AppErr    { return &AppErr{413, "payload_too_large", "Upload too large"} }
func errQuota() *AppErr              { return &AppErr{507, "quota_exceeded", "Storage quota exceeded"} }
func errBusy() *AppErr               { return &AppErr{503, "busy", "Server busy; retry shortly"} }
func errInternal(e error) *AppErr {
	log.Printf("internal error: %v", e)
	return &AppErr{500, "internal_error", "Internal server error"}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, e error) {
	var ae *AppErr
	if errors.As(e, &ae) {
		writeJSON(w, ae.Status, map[string]string{"error": ae.Code, "message": ae.Msg})
		return
	}
	writeJSON(w, 500, map[string]string{"error": "internal_error", "message": "Internal server error"})
}

// ---------------------------------------------------------------------------
// Database
// ---------------------------------------------------------------------------

func openDB(path string) (*sql.DB, error) {
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=busy_timeout(10000)" +
		"&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			quota_bytes INTEGER NOT NULL DEFAULT 0,
			used_bytes INTEGER NOT NULL DEFAULT 0,
			disabled INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			csrf TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			ip TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS files (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			parent_id TEXT,
			name TEXT NOT NULL,
			is_dir INTEGER NOT NULL DEFAULT 0,
			storage_path TEXT,
			size INTEGER NOT NULL DEFAULT 0,
			mime TEXT NOT NULL DEFAULT 'application/octet-stream',
			sha256 TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_files_user_parent ON files(user_id, parent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_files_user_name ON files(user_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	defaults := map[string]string{
		"server_display_name": "HomeLite",
		"max_upload_bytes":    strconv.FormatInt(defaultMaxUpload, 10),
		"default_quota_bytes": strconv.FormatInt(defaultQuota, 10),
		"session_hours":       strconv.FormatInt(defaultSessionHours, 10),
		"registration_open":   "false",
		"theme":               "system",
		"port":                strconv.Itoa(defaultPort),
	}
	for k, v := range defaults {
		if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES(?,?)`, k, v); err != nil {
			return err
		}
	}
	return nil
}

func getSetting(db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func setSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func getIntSetting(db *sql.DB, key string, def int64) int64 {
	v, err := getSetting(db, key)
	if err != nil || v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

// ---------------------------------------------------------------------------
// Password / tokens
// ---------------------------------------------------------------------------

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		hex.EncodeToString(salt), hex.EncodeToString(key)), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := hex.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func genToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func uuidV4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

type AuthCtx struct {
	User User
	CSRF string
}

func loadUser(db *sql.DB, id int64) (User, error) {
	var u User
	var created string
	var disabled int
	err := db.QueryRow(`SELECT id,username,role,quota_bytes,used_bytes,disabled,created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Username, &u.Role, &u.QuotaBytes, &u.UsedBytes, &disabled, &created)
	if err != nil {
		return u, err
	}
	u.Disabled = disabled != 0
	u.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return u, nil
}

func cookieToken(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func authFromRequest(db *sql.DB, r *http.Request) (*AuthCtx, error) {
	token := cookieToken(r)
	if token == "" {
		return nil, nil
	}
	var userID int64
	var csrf, expires string
	err := db.QueryRow(`SELECT user_id, csrf, expires_at FROM sessions WHERE token=?`, token).
		Scan(&userID, &csrf, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	exp, err := time.Parse(time.RFC3339, expires)
	if err != nil || exp.Before(time.Now()) {
		_, _ = db.Exec(`DELETE FROM sessions WHERE token=?`, token)
		return nil, nil
	}
	u, err := loadUser(db, userID)
	if err != nil {
		return nil, err
	}
	if u.Disabled {
		return nil, nil
	}
	return &AuthCtx{User: u, CSRF: csrf}, nil
}

func requireAuth(db *sql.DB, r *http.Request) (*AuthCtx, error) {
	ctx, err := authFromRequest(db, r)
	if err != nil {
		return nil, errInternal(err)
	}
	if ctx == nil {
		return nil, errUnauthorized()
	}
	return ctx, nil
}

func requireCSRF(r *http.Request, expected string) error {
	got := r.Header.Get(csrfHeader)
	if got == "" {
		return errForbidden()
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		return errForbidden()
	}
	return nil
}

func isHTTPS(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") || r.TLS != nil
}

func setSessionCookie(w http.ResponseWriter, token string, maxAge int, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   maxAge,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]{3,64}$`)

func validateCredentials(username, password string) error {
	if !usernameRe.MatchString(username) {
		return errBadRequest("username must be 3–64 chars: letters, digits, _ - .")
	}
	if len(password) < 8 {
		return errBadRequest("password must be at least 8 characters")
	}
	if len(password) > 1024 {
		return errBadRequest("password too long")
	}
	return nil
}

func sanitizeFilename(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" || len(n) > 255 {
		return "", errBadRequest("invalid filename")
	}
	if strings.ContainsAny(n, "/\\\x00") {
		return "", errBadRequest("filename contains illegal characters")
	}
	if n == "." || n == ".." {
		return "", errBadRequest("invalid filename")
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Handlers: status / setup / auth
// ---------------------------------------------------------------------------

func serverNeedsSetup(db *sql.DB) bool {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&n)
	return n == 0
}

func handleStatus(db *sql.DB, st *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var files, users, totalSize int64
		_ = db.QueryRow(`SELECT COUNT(*) FROM files WHERE is_dir=0`).Scan(&files)
		_ = db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users)
		_ = db.QueryRow(`SELECT COALESCE(SUM(size),0) FROM files WHERE is_dir=0`).Scan(&totalSize)
		display, _ := getSetting(db, "server_display_name")
		if display == "" {
			display = "HomeLite"
		}
		writeJSON(w, 200, map[string]any{
			"name":             display,
			"needs_setup":      serverNeedsSetup(db),
			"files":            files,
			"users":            users,
			"total_size":       totalSize,
			"uptime_secs":      int64(time.Since(st.startedAt).Seconds()),
			"lan_ip":           localIP(),
			"port":             getIntSetting(db, "port", defaultPort),
			"bytes_served":     st.bytesServed.Load(),
			"max_upload_bytes": st.maxUploadBytes.Load(),
		})
	}
}

func handleSetup(db *sql.DB) http.HandlerFunc {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !serverNeedsSetup(db) {
			writeErr(w, errForbidden())
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, errBadRequest("invalid JSON"))
			return
		}
		if err := validateCredentials(body.Username, body.Password); err != nil {
			writeErr(w, err)
			return
		}
		hash, err := hashPassword(body.Password)
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		quota := getIntSetting(db, "default_quota_bytes", defaultQuota)
		_, err = db.Exec(`INSERT INTO users(username,password_hash,role,quota_bytes,used_bytes,disabled,created_at) VALUES(?,?,?,?,0,0,?)`,
			body.Username, hash, "admin", quota, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		log.Printf("administrator account created: %s", body.Username)
		writeJSON(w, 201, map[string]any{"ok": true})
	}
}

func handleLogin(db *sql.DB, st *AppState) http.HandlerFunc {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, errBadRequest("invalid JSON"))
			return
		}

		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		key := body.Username + "|" + ip

		if v, ok := st.loginAttempts.Load(key); ok {
			e := v.(*attemptEntry)
			e.mu.Lock()
			blocked := time.Since(e.first) < loginWindow && e.count >= loginMaxAttempts
			e.mu.Unlock()
			if blocked {
				writeErr(w, errTooMany())
				return
			}
		}

		var (
			id           int64
			passwordHash string
			disabled     int
		)
		err := db.QueryRow(`SELECT id, password_hash, disabled FROM users WHERE username=?`, body.Username).
			Scan(&id, &passwordHash, &disabled)
		ok := false
		if err == nil && disabled == 0 {
			ok = verifyPassword(body.Password, passwordHash)
		} else {
			_ = verifyPassword(body.Password,
				"$argon2id$v=19$m=65536,t=2,p=4$00000000000000000000000000000000$0000000000000000000000000000000000000000000000000000000000000000")
		}

		if !ok {
			v, _ := st.loginAttempts.LoadOrStore(key, &attemptEntry{first: time.Now()})
			e := v.(*attemptEntry)
			e.mu.Lock()
			if time.Since(e.first) >= loginWindow {
				e.count = 0
				e.first = time.Now()
			}
			e.count++
			e.mu.Unlock()
			writeErr(w, errUnauthorized())
			return
		}

		st.loginAttempts.Delete(key)

		token := genToken()
		csrf := genToken()
		hours := getIntSetting(db, "session_hours", defaultSessionHours)
		expires := time.Now().Add(time.Duration(hours) * time.Hour)

		_, err = db.Exec(`INSERT INTO sessions(token,user_id,csrf,created_at,expires_at,ip) VALUES(?,?,?,?,?,?)`,
			token, id, csrf, time.Now().UTC().Format(time.RFC3339), expires.UTC().Format(time.RFC3339), ip)
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}

		setSessionCookie(w, token, int(hours*3600), isHTTPS(r))
		writeJSON(w, 200, map[string]any{"ok": true, "csrf": csrf})
	}
}

func handleLogout(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token := cookieToken(r); token != "" {
			_, _ = db.Exec(`DELETE FROM sessions WHERE token=?`, token)
		}
		clearSessionCookie(w)
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

func handleMe(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"user": ctx.User, "csrf": ctx.CSRF})
	}
}

func handleChangePassword(db *sql.DB) http.HandlerFunc {
	type req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := requireCSRF(r, ctx.CSRF); err != nil {
			writeErr(w, err)
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, errBadRequest("invalid JSON"))
			return
		}
		var hash string
		if err := db.QueryRow(`SELECT password_hash FROM users WHERE id=?`, ctx.User.ID).Scan(&hash); err != nil {
			writeErr(w, errInternal(err))
			return
		}
		if !verifyPassword(body.OldPassword, hash) {
			writeErr(w, errUnauthorized())
			return
		}
		if err := validateCredentials(ctx.User.Username, body.NewPassword); err != nil {
			writeErr(w, err)
			return
		}
		newHash, err := hashPassword(body.NewPassword)
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		tx, err := db.Begin()
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`UPDATE users SET password_hash=? WHERE id=?`, newHash, ctx.User.ID); err != nil {
			writeErr(w, errInternal(err))
			return
		}
		if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id=?`, ctx.User.ID); err != nil {
			writeErr(w, errInternal(err))
			return
		}
		if err := tx.Commit(); err != nil {
			writeErr(w, errInternal(err))
			return
		}
		clearSessionCookie(w)
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

// ---------------------------------------------------------------------------
// Admin handlers
// ---------------------------------------------------------------------------

func handleAdminListUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if ctx.User.Role != "admin" {
			writeErr(w, errForbidden())
			return
		}
		rows, err := db.Query(`SELECT id,username,role,quota_bytes,used_bytes,disabled,created_at FROM users ORDER BY id`)
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		defer rows.Close()
		out := []User{}
		for rows.Next() {
			var u User
			var created string
			var disabled int
			if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.QuotaBytes, &u.UsedBytes, &disabled, &created); err != nil {
				writeErr(w, errInternal(err))
				return
			}
			u.Disabled = disabled != 0
			u.CreatedAt, _ = time.Parse(time.RFC3339, created)
			out = append(out, u)
		}
		writeJSON(w, 200, map[string]any{"users": out})
	}
}

func handleAdminCreateUser(db *sql.DB) http.HandlerFunc {
	type req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		Role       string `json:"role"`
		QuotaBytes int64  `json:"quota_bytes"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if ctx.User.Role != "admin" {
			writeErr(w, errForbidden())
			return
		}
		if err := requireCSRF(r, ctx.CSRF); err != nil {
			writeErr(w, err)
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, errBadRequest("invalid JSON"))
			return
		}
		if err := validateCredentials(body.Username, body.Password); err != nil {
			writeErr(w, err)
			return
		}
		role := "user"
		if body.Role == "admin" {
			role = "admin"
		}
		quota := body.QuotaBytes
		if quota <= 0 {
			quota = getIntSetting(db, "default_quota_bytes", defaultQuota)
		}
		hash, err := hashPassword(body.Password)
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		_, err = db.Exec(`INSERT INTO users(username,password_hash,role,quota_bytes,used_bytes,disabled,created_at) VALUES(?,?,?,?,0,0,?)`,
			body.Username, hash, role, quota, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				writeErr(w, errConflict("username taken"))
				return
			}
			writeErr(w, errInternal(err))
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

func handleAdminPatchUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if ctx.User.Role != "admin" {
			writeErr(w, errForbidden())
			return
		}
		if err := requireCSRF(r, ctx.CSRF); err != nil {
			writeErr(w, err)
			return
		}
		idStr := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
		idStr = strings.TrimSuffix(idStr, "/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeErr(w, errBadRequest("invalid user id"))
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, errBadRequest("invalid JSON"))
			return
		}
		if id == ctx.User.ID {
			if d, ok := body["disabled"].(bool); ok && d {
				writeErr(w, errBadRequest("cannot disable yourself"))
				return
			}
		}
		tx, err := db.Begin()
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		defer tx.Rollback()
		if d, ok := body["disabled"].(bool); ok {
			flag := 0
			if d {
				flag = 1
			}
			if _, err := tx.Exec(`UPDATE users SET disabled=? WHERE id=?`, flag, id); err != nil {
				writeErr(w, errInternal(err))
				return
			}
			if d {
				if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id=?`, id); err != nil {
					writeErr(w, errInternal(err))
					return
				}
			}
		}
		if q, ok := body["quota_bytes"].(float64); ok {
			if _, err := tx.Exec(`UPDATE users SET quota_bytes=? WHERE id=?`, int64(q), id); err != nil {
				writeErr(w, errInternal(err))
				return
			}
		}
		if role, ok := body["role"].(string); ok && (role == "admin" || role == "user") {
			if _, err := tx.Exec(`UPDATE users SET role=? WHERE id=?`, role, id); err != nil {
				writeErr(w, errInternal(err))
				return
			}
		}
		if np, ok := body["new_password"].(string); ok && np != "" {
			if len(np) < 8 {
				writeErr(w, errBadRequest("new password too short"))
				return
			}
			hash, err := hashPassword(np)
			if err != nil {
				writeErr(w, errInternal(err))
				return
			}
			if _, err := tx.Exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, id); err != nil {
				writeErr(w, errInternal(err))
				return
			}
			if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id=?`, id); err != nil {
				writeErr(w, errInternal(err))
				return
			}
		}
		if err := tx.Commit(); err != nil {
			writeErr(w, errInternal(err))
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

func handleAdminDeleteUser(db *sql.DB, st *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if ctx.User.Role != "admin" {
			writeErr(w, errForbidden())
			return
		}
		if err := requireCSRF(r, ctx.CSRF); err != nil {
			writeErr(w, err)
			return
		}
		idStr := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
		idStr = strings.TrimSuffix(idStr, "/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeErr(w, errBadRequest("invalid user id"))
			return
		}
		if id == ctx.User.ID {
			writeErr(w, errBadRequest("cannot delete yourself"))
			return
		}
		rows, err := db.Query(`SELECT storage_path FROM files WHERE user_id=? AND is_dir=0 AND storage_path IS NOT NULL`, id)
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		var paths []string
		for rows.Next() {
			var p string
			_ = rows.Scan(&p)
			paths = append(paths, p)
		}
		rows.Close()
		if _, err := db.Exec(`DELETE FROM users WHERE id=?`, id); err != nil {
			writeErr(w, errInternal(err))
			return
		}
		for _, p := range paths {
			_ = os.Remove(filepath.Join(st.root, filepath.FromSlash(p)))
		}
		_ = os.RemoveAll(st.filesDirFor(id))
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

func handleAdminGetSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if ctx.User.Role != "admin" {
			writeErr(w, errForbidden())
			return
		}
		keys := []string{"server_display_name", "max_upload_bytes", "default_quota_bytes", "session_hours", "registration_open", "theme", "port"}
		out := map[string]string{}
		for _, k := range keys {
			v, _ := getSetting(db, k)
			out[k] = v
		}
		writeJSON(w, 200, out)
	}
}

func handleAdminPutSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if ctx.User.Role != "admin" {
			writeErr(w, errForbidden())
			return
		}
		if err := requireCSRF(r, ctx.CSRF); err != nil {
			writeErr(w, err)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, errBadRequest("invalid JSON"))
			return
		}
		if v, ok := body["server_display_name"].(string); ok {
			if len(v) > 64 {
				writeErr(w, errBadRequest("display name too long"))
				return
			}
			_ = setSetting(db, "server_display_name", v)
		}
		if v, ok := body["max_upload_bytes"].(float64); ok {
			if v < 1024 {
				writeErr(w, errBadRequest("max_upload_bytes too small"))
				return
			}
			_ = setSetting(db, "max_upload_bytes", strconv.FormatInt(int64(v), 10))
		}
		if v, ok := body["default_quota_bytes"].(float64); ok {
			if v < 0 {
				writeErr(w, errBadRequest("default_quota_bytes must be >= 0"))
				return
			}
			_ = setSetting(db, "default_quota_bytes", strconv.FormatInt(int64(v), 10))
		}
		if v, ok := body["session_hours"].(float64); ok {
			if v < 1 || v > 24*365 {
				writeErr(w, errBadRequest("session_hours out of range"))
				return
			}
			_ = setSetting(db, "session_hours", strconv.FormatInt(int64(v), 10))
		}
		if v, ok := body["registration_open"].(bool); ok {
			val := "false"
			if v {
				val = "true"
			}
			_ = setSetting(db, "registration_open", val)
		}
		if v, ok := body["theme"].(string); ok {
			_ = setSetting(db, "theme", v)
		}
		if v, ok := body["port"].(float64); ok {
			if v < 1 || v > 65535 {
				writeErr(w, errBadRequest("invalid port"))
				return
			}
			_ = setSetting(db, "port", strconv.FormatInt(int64(v), 10))
		}
		writeJSON(w, 200, map[string]any{"ok": true, "restart_required_for": []string{"port"}})
	}
}

// ---------------------------------------------------------------------------
// Files
// ---------------------------------------------------------------------------

func fetchEntry(db *sql.DB, userID int64, id string) (FileEntry, error) {
	var f FileEntry
	var parent sql.NullString
	var isDir int
	var created, updated string
	err := db.QueryRow(`SELECT id,parent_id,name,is_dir,size,mime,created_at,updated_at FROM files WHERE id=? AND user_id=?`, id, userID).
		Scan(&f.ID, &parent, &f.Name, &isDir, &f.Size, &f.Mime, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return f, errNotFound()
	}
	if err != nil {
		return f, err
	}
	if parent.Valid {
		f.ParentID = &parent.String
	}
	f.IsDir = isDir != 0
	f.CreatedAt, _ = time.Parse(time.RFC3339, created)
	f.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return f, nil
}

func handleFilesList(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		q := r.URL.Query()
		sortBy := "name"
		switch q.Get("sort") {
		case "size":
			sortBy = "size"
		case "date":
			sortBy = "updated_at"
		}
		order := "ASC"
		if strings.EqualFold(q.Get("order"), "desc") {
			order = "DESC"
		}
		var rows *sql.Rows
		searchTerm := q.Get("q")
		parentID := q.Get("parent_id")

		switch {
		case searchTerm != "":
			sqlStr := fmt.Sprintf(`SELECT id,parent_id,name,is_dir,size,mime,created_at,updated_at FROM files WHERE user_id=? AND name LIKE ? ORDER BY is_dir DESC, %s %s`, sortBy, order)
			rows, err = db.Query(sqlStr, ctx.User.ID, "%"+searchTerm+"%")
		case parentID != "":
			sqlStr := fmt.Sprintf(`SELECT id,parent_id,name,is_dir,size,mime,created_at,updated_at FROM files WHERE user_id=? AND parent_id=? ORDER BY is_dir DESC, %s %s`, sortBy, order)
			rows, err = db.Query(sqlStr, ctx.User.ID, parentID)
		default:
			sqlStr := fmt.Sprintf(`SELECT id,parent_id,name,is_dir,size,mime,created_at,updated_at FROM files WHERE user_id=? AND parent_id IS NULL ORDER BY is_dir DESC, %s %s`, sortBy, order)
			rows, err = db.Query(sqlStr, ctx.User.ID)
		}
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		defer rows.Close()

		out := []FileEntry{}
		for rows.Next() {
			var f FileEntry
			var parent sql.NullString
			var isDir int
			var created, updated string
			if err := rows.Scan(&f.ID, &parent, &f.Name, &isDir, &f.Size, &f.Mime, &created, &updated); err != nil {
				writeErr(w, errInternal(err))
				return
			}
			if parent.Valid {
				f.ParentID = &parent.String
			}
			f.IsDir = isDir != 0
			f.CreatedAt, _ = time.Parse(time.RFC3339, created)
			f.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
			out = append(out, f)
		}
		writeJSON(w, 200, map[string]any{"files": out, "user": ctx.User})
	}
}

func handleFilesMkdir(db *sql.DB) http.HandlerFunc {
	type req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := requireCSRF(r, ctx.CSRF); err != nil {
			writeErr(w, err)
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, errBadRequest("invalid JSON"))
			return
		}
		name, err := sanitizeFilename(body.Name)
		if err != nil {
			writeErr(w, err)
			return
		}
		var parentPtr *string
		if body.ParentID != "" {
			parent, err := fetchEntry(db, ctx.User.ID, body.ParentID)
			if err != nil {
				writeErr(w, err)
				return
			}
			if !parent.IsDir {
				writeErr(w, errBadRequest("parent is not a folder"))
				return
			}
			pid := body.ParentID
			parentPtr = &pid
		}
		id := uuidV4()
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = db.Exec(`INSERT INTO files(id,user_id,parent_id,name,is_dir,storage_path,size,mime,created_at,updated_at)
			VALUES(?,?,?,?,1,NULL,0,'inode/directory',?,?)`, id, ctx.User.ID, parentPtr, name, now, now)
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "id": id})
	}
}

func handleFilesRename(db *sql.DB) http.HandlerFunc {
	type req struct {
		Name string `json:"name"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := requireCSRF(r, ctx.CSRF); err != nil {
			writeErr(w, err)
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/files/"), "/rename")
		if _, err := fetchEntry(db, ctx.User.ID, id); err != nil {
			writeErr(w, err)
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, errBadRequest("invalid JSON"))
			return
		}
		name, err := sanitizeFilename(body.Name)
		if err != nil {
			writeErr(w, err)
			return
		}
		_, err = db.Exec(`UPDATE files SET name=?, updated_at=? WHERE id=? AND user_id=?`,
			name, time.Now().UTC().Format(time.RFC3339), id, ctx.User.ID)
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

func handleFilesMove(db *sql.DB) http.HandlerFunc {
	type req struct {
		ParentID *string `json:"parent_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := requireCSRF(r, ctx.CSRF); err != nil {
			writeErr(w, err)
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/files/"), "/move")
		if _, err := fetchEntry(db, ctx.User.ID, id); err != nil {
			writeErr(w, err)
			return
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, errBadRequest("invalid JSON"))
			return
		}
		if body.ParentID != nil && *body.ParentID != "" {
			if *body.ParentID == id {
				writeErr(w, errBadRequest("cannot move into itself"))
				return
			}
			parent, err := fetchEntry(db, ctx.User.ID, *body.ParentID)
			if err != nil {
				writeErr(w, err)
				return
			}
			if !parent.IsDir {
				writeErr(w, errBadRequest("target is not a folder"))
				return
			}
			cur := parent.ParentID
			for cur != nil {
				if *cur == id {
					writeErr(w, errBadRequest("cannot move into descendant"))
					return
				}
				e, err := fetchEntry(db, ctx.User.ID, *cur)
				if err != nil {
					break
				}
				cur = e.ParentID
			}
		} else {
			body.ParentID = nil
		}
		_, err = db.Exec(`UPDATE files SET parent_id=?, updated_at=? WHERE id=? AND user_id=?`,
			body.ParentID, time.Now().UTC().Format(time.RFC3339), id, ctx.User.ID)
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

func handleFilesDelete(db *sql.DB, st *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := requireCSRF(r, ctx.CSRF); err != nil {
			writeErr(w, err)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/files/")
		entry, err := fetchEntry(db, ctx.User.ID, id)
		if err != nil {
			writeErr(w, err)
			return
		}

		if entry.IsDir {
			toDelete := []string{id}
			i := 0
			for i < len(toDelete) {
				cur := toDelete[i]
				i++
				rows, err := db.Query(`SELECT id FROM files WHERE user_id=? AND parent_id=?`, ctx.User.ID, cur)
				if err != nil {
					break
				}
				for rows.Next() {
					var cid string
					_ = rows.Scan(&cid)
					toDelete = append(toDelete, cid)
				}
				rows.Close()
			}

			var paths []string
			var totalSize int64
			for _, fid := range toDelete {
				var p sql.NullString
				var sz int64
				if err := db.QueryRow(`SELECT storage_path, COALESCE(size,0) FROM files WHERE id=? AND user_id=?`, fid, ctx.User.ID).Scan(&p, &sz); err == nil {
					if p.Valid {
						paths = append(paths, p.String)
					}
					totalSize += sz
				}
			}

			tx, err := db.Begin()
			if err != nil {
				writeErr(w, errInternal(err))
				return
			}
			for _, fid := range toDelete {
				if _, err := tx.Exec(`DELETE FROM files WHERE id=? AND user_id=?`, fid, ctx.User.ID); err != nil {
					tx.Rollback()
					writeErr(w, errInternal(err))
					return
				}
			}
			if _, err := tx.Exec(`UPDATE users SET used_bytes = MAX(0, used_bytes - ?) WHERE id=?`, totalSize, ctx.User.ID); err != nil {
				tx.Rollback()
				writeErr(w, errInternal(err))
				return
			}
			if err := tx.Commit(); err != nil {
				writeErr(w, errInternal(err))
				return
			}
			for _, p := range paths {
				_ = os.Remove(filepath.Join(st.root, filepath.FromSlash(p)))
			}
		} else {
			var p sql.NullString
			if err := db.QueryRow(`SELECT storage_path FROM files WHERE id=? AND user_id=?`, id, ctx.User.ID).Scan(&p); err != nil && !errors.Is(err, sql.ErrNoRows) {
				writeErr(w, errInternal(err))
				return
			}
			tx, err := db.Begin()
			if err != nil {
				writeErr(w, errInternal(err))
				return
			}
			if _, err := tx.Exec(`DELETE FROM files WHERE id=? AND user_id=?`, id, ctx.User.ID); err != nil {
				tx.Rollback()
				writeErr(w, errInternal(err))
				return
			}
			if _, err := tx.Exec(`UPDATE users SET used_bytes = MAX(0, used_bytes - ?) WHERE id=?`, entry.Size, ctx.User.ID); err != nil {
				tx.Rollback()
				writeErr(w, errInternal(err))
				return
			}
			if err := tx.Commit(); err != nil {
				writeErr(w, errInternal(err))
				return
			}
			if p.Valid {
				_ = os.Remove(filepath.Join(st.root, filepath.FromSlash(p.String)))
			}
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

func handleFilesDownload(db *sql.DB, st *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/files/"), "/download")
		var name, storagePath, mimeType string
		var size int64
		err = db.QueryRow(`SELECT name, storage_path, mime, size FROM files WHERE id=? AND user_id=? AND is_dir=0`, id, ctx.User.ID).
			Scan(&name, &storagePath, &mimeType, &size)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, errNotFound())
			return
		}
		if err != nil {
			writeErr(w, errInternal(err))
			return
		}
		full := filepath.Join(st.root, filepath.FromSlash(storagePath))
		f, err := os.Open(full)
		if err != nil {
			writeErr(w, errNotFound())
			return
		}
		defer f.Close()

		st.bytesServed.Add(size)
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		safeName := strings.ReplaceAll(name, `"`, "")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
		_, _ = io.Copy(w, f)
	}
}

func handleFilesUpload(db *sql.DB, st *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := requireAuth(db, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := requireCSRF(r, ctx.CSRF); err != nil {
			writeErr(w, err)
			return
		}

		select {
		case st.uploadSem <- struct{}{}:
			defer func() { <-st.uploadSem }()
		default:
			writeErr(w, errBusy())
			return
		}

		reader, err := r.MultipartReader()
		if err != nil {
			writeErr(w, errBadRequest("expected multipart/form-data"))
			return
		}

		var parentID string
		var fileSaved bool
		var fileName string
		var tmpPath string
		var written int64
		hasher := sha256.New()
		maxBytes := st.maxUploadBytes.Load()

		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				writeErr(w, errBadRequest("multipart parse: "+err.Error()))
				return
			}
			name := part.FormName()
			if name == "parent_id" {
				buf, _ := io.ReadAll(io.LimitReader(part, 256))
				parentID = strings.TrimSpace(string(buf))
				part.Close()
				continue
			}
			if name == "file" && !fileSaved {
				raw := part.FileName()
				safe, err := sanitizeFilename(raw)
				if err != nil {
					part.Close()
					writeErr(w, err)
					return
				}
				fileName = safe
				tmpPath = filepath.Join(st.tempDir, "up-"+uuidV4())
				out, err := os.Create(tmpPath)
				if err != nil {
					part.Close()
					writeErr(w, errInternal(err))
					return
				}
				buf := make([]byte, 256*1024)
				for {
					n, rerr := part.Read(buf)
					if n > 0 {
						written += int64(n)
						if written > maxBytes {
							out.Close()
							_ = os.Remove(tmpPath)
							part.Close()
							writeErr(w, errPayloadTooLarge())
							return
						}
						hasher.Write(buf[:n])
						if _, werr := out.Write(buf[:n]); werr != nil {
							out.Close()
							_ = os.Remove(tmpPath)
							part.Close()
							writeErr(w, errInternal(werr))
							return
						}
					}
					if rerr == io.EOF {
						break
					}
					if rerr != nil {
						out.Close()
						_ = os.Remove(tmpPath)
						part.Close()
						writeErr(w, errBadRequest("upload stream: "+rerr.Error()))
						return
					}
				}
				if err := out.Sync(); err != nil {
					out.Close()
					_ = os.Remove(tmpPath)
					part.Close()
					writeErr(w, errInternal(err))
					return
				}
				out.Close()
				part.Close()
				fileSaved = true
				continue
			}
			_, _ = io.Copy(io.Discard, part)
			part.Close()
		}

		if !fileSaved || tmpPath == "" {
			writeErr(w, errBadRequest("missing file field"))
			return
		}

		var parentPtr *string
		if parentID != "" {
			parent, err := fetchEntry(db, ctx.User.ID, parentID)
			if err != nil {
				_ = os.Remove(tmpPath)
				writeErr(w, err)
				return
			}
			if !parent.IsDir {
				_ = os.Remove(tmpPath)
				writeErr(w, errBadRequest("parent is not a folder"))
				return
			}
			pid := parentID
			parentPtr = &pid
		}

		fresh, err := loadUser(db, ctx.User.ID)
		if err != nil {
			_ = os.Remove(tmpPath)
			writeErr(w, errInternal(err))
			return
		}
		if fresh.QuotaBytes > 0 && fresh.UsedBytes+written > fresh.QuotaBytes {
			_ = os.Remove(tmpPath)
			writeErr(w, errQuota())
			return
		}

		fileID := uuidV4()
		userDir := st.filesDirFor(ctx.User.ID)
		if err := os.MkdirAll(userDir, 0o755); err != nil {
			_ = os.Remove(tmpPath)
			writeErr(w, errInternal(err))
			return
		}
		finalPath := filepath.Join(userDir, fileID)
		if err := os.Rename(tmpPath, finalPath); err != nil {
			_ = os.Remove(tmpPath)
			writeErr(w, errInternal(err))
			return
		}

		rel := "files/" + strconv.FormatInt(ctx.User.ID, 10) + "/" + fileID
		mimeType := mime.TypeByExtension(filepath.Ext(fileName))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		sha := hex.EncodeToString(hasher.Sum(nil))
		now := time.Now().UTC().Format(time.RFC3339)

		tx, err := db.Begin()
		if err != nil {
			_ = os.Remove(finalPath)
			writeErr(w, errInternal(err))
			return
		}
		if _, err := tx.Exec(`INSERT INTO files(id,user_id,parent_id,name,is_dir,storage_path,size,mime,sha256,created_at,updated_at)
			VALUES(?,?,?,?,0,?,?,?,?,?,?)`,
			fileID, ctx.User.ID, parentPtr, fileName, rel, written, mimeType, sha, now, now); err != nil {
			tx.Rollback()
			_ = os.Remove(finalPath)
			writeErr(w, errInternal(err))
			return
		}
		if _, err := tx.Exec(`UPDATE users SET used_bytes = used_bytes + ? WHERE id=?`, written, ctx.User.ID); err != nil {
			tx.Rollback()
			_ = os.Remove(finalPath)
			writeErr(w, errInternal(err))
			return
		}
		if err := tx.Commit(); err != nil {
			_ = os.Remove(finalPath)
			writeErr(w, errInternal(err))
			return
		}

		writeJSON(w, 200, map[string]any{
			"ok":     true,
			"id":     fileID,
			"name":   fileName,
			"size":   written,
			"sha256": sha,
		})
	}
}

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

func buildRouter(db *sql.DB, st *AppState) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			data, err := assets.ReadFile("index.html")
			if err != nil {
				http.Error(w, "dashboard asset missing", 500)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("/api/status", handleStatus(db, st))
	mux.HandleFunc("/api/setup", handleSetup(db))
	mux.HandleFunc("/api/login", handleLogin(db, st))
	mux.HandleFunc("/api/logout", handleLogout(db))
	mux.HandleFunc("/api/me", handleMe(db))
	mux.HandleFunc("/api/me/password", handleChangePassword(db))

	mux.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleFilesList(db)(w, r)
			return
		}
		http.Error(w, "method not allowed", 405)
	})
	mux.HandleFunc("/api/files/mkdir", handleFilesMkdir(db))
	mux.HandleFunc("/api/files/upload", handleFilesUpload(db, st))
	mux.HandleFunc("/api/files/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/files/")
		switch {
		case r.Method == http.MethodDelete && !strings.Contains(path, "/"):
			handleFilesDelete(db, st)(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/rename"):
			handleFilesRename(db)(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/move"):
			handleFilesMove(db)(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/download"):
			handleFilesDownload(db, st)(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleAdminListUsers(db)(w, r)
		case http.MethodPost:
			handleAdminCreateUser(db)(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/admin/users/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch, http.MethodPut:
			handleAdminPatchUser(db)(w, r)
		case http.MethodDelete:
			handleAdminDeleteUser(db, st)(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/admin/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleAdminGetSettings(db)(w, r)
		case http.MethodPut, http.MethodPost:
			handleAdminPutSettings(db)(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})

	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return "127.0.0.1"
}

func humanBytes(n int64) string {
	f := float64(n)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[i])
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

func resolveDataDir() string {
	if v := os.Getenv("HOMELITE_DATA_DIR"); v != "" {
		return v
	}
	return "data"
}

func reconcileTemp(tempDir string) {
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return
	}
	removed := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "up-") {
			if err := os.Remove(filepath.Join(tempDir, e.Name())); err == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		log.Printf("removed %d orphan temporary upload(s)", removed)
	}
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	noAnim := os.Getenv("HOMELITE_NO_ANIM") != ""
	for _, a := range os.Args[1:] {
		if a == "--no-anim" {
			noAnim = true
		}
	}
	if !noAnim {
		playSplash()
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("homelite ")

	root, _ := filepath.Abs(resolveDataDir())
	filesDir := filepath.Join(root, "files")
	tempDir := filepath.Join(root, "temp")
	logsDir := filepath.Join(root, "logs")
	for _, d := range []string{root, filesDir, tempDir, logsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Fatalf("create %s: %v", d, err)
		}
	}

	dbPath := filepath.Join(root, "homelite.db")
	db, err := openDB(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := initSchema(db); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	reconcileTemp(tempDir)

	maxUpload := getIntSetting(db, "max_upload_bytes", defaultMaxUpload)
	st := &AppState{
		db:        db,
		root:      root,
		filesDir:  filesDir,
		tempDir:   tempDir,
		uploadSem: make(chan struct{}, maxConcurrentUpload),
		startedAt: time.Now(),
	}
	st.maxUploadBytes.Store(maxUpload)

	port := int(getIntSetting(db, "port", defaultPort))
	addr := fmt.Sprintf("%s:%d", defaultHost, port)

	handler := buildRouter(db, st)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("bind %s: %v", addr, err)
	}

	display, _ := getSetting(db, "server_display_name")
	if display == "" {
		display = "HomeLite"
	}
	lan := localIP()
	fmt.Printf("%s%s┌─────────────────────────────────────────────────────────────┐%s\n", ansiCyan, ansiBold, ansiReset)
	fmt.Printf("%s%s│  %s is listening%s\n", ansiCyan, ansiBold, display, ansiReset)
	fmt.Printf("%s%s│  local:  http://127.0.0.1:%d%s\n", ansiCyan, ansiBold, port, ansiReset)
	fmt.Printf("%s%s│  LAN:    http://%s:%d%s\n", ansiCyan, ansiBold, lan, port, ansiReset)
	fmt.Printf("%s%s│  data:   %s%s\n", ansiCyan, ansiBold, root, ansiReset)
	fmt.Printf("%s%s│  press Ctrl-C to stop%s\n", ansiCyan, ansiBold, ansiReset)
	fmt.Printf("%s%s└─────────────────────────────────────────────────────────────┘%s\n", ansiCyan, ansiBold, ansiReset)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		fmt.Printf("%s%sshutting down…%s\n", ansiYellow, ansiBold, ansiReset)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}

	_, _ = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	log.Printf("goodbye (served %s total)", humanBytes(st.bytesServed.Load()))
}