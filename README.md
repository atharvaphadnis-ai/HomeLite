## Homelite


https://github.com/user-attachments/assets/27ace32b-3572-4f6d-b05c-07ce325b7cc6


## Screenshots
<img width="1366" height="768" alt="Screenshot (191)" src="https://github.com/user-attachments/assets/ef60aefb-c7f7-47f6-bcb6-182311f005d0" />


<img width="1366" height="768" alt="Screenshot (191)" src="https://github.com/user-attachments/assets/6ebaeaa7-5217-482d-b0ae-a4467dc33a7d" />


<img width="1366" height="768" alt="Screenshot (193)" src="https://github.com/user-attachments/assets/3cdd5487-4d50-4f41-8695-cf4791ea59ec" />


## Quick Start

There are two paths: download a prebuilt binary, or build from source. Both produce the same single executable.

### Option A — Prebuilt Binary

1. Download the latest release for your platform from the project's releases page.
2. Place the binary anywhere on your `PATH`, or run it in place.

Linux / macOS:

```bash
chmod +x homelite
./homelite
```

Windows (Command Prompt):

```cmd
homelite.exe
```

3. Open the URL printed in the terminal splash (typically `http://localhost:8080`).
4. Complete the first-run setup page to create your administrator account.
5. Verify the server is reachable from another device on the same network using the LAN URL printed in the splash. See [Accessing From Other Devices](#accessing-from-other-devices) if it is not.

### Option B — Build From Source

Requires Go 1.22 or later.

```bash
git clone https://example.com/homelite.git
cd homelite
go build -o homelite .
./homelite
```

See [Building From Source](#building-from-source) for release builds, dependency management, and cross-compilation.

### Verifying the Install

After starting the server, the splash prints something like:

```text
HomeLite 1.0.0
Local:  http://localhost:8080
LAN:    http://192.168.1.42:8080
Data:   /home/you/homelite/data
```

If `LAN:` is missing, the server could not detect a non-loopback interface. It is still reachable on `localhost`. Check your network configuration if you need LAN access.

## Building From Source

HomeLite uses only the Go standard library and a small number of pure-Go third-party packages. There is no CGO, so no C compiler is required on any platform.

### Prerequisites

- Go 1.22 or later. Verify with `go version`.
- Git (to fetch the source).

### Fetch and Build (Development)

```bash
git clone https://example.com/homelite.git
cd homelite
go mod download
go build -o homelite .
./homelite
```

The development build includes the symbol table and is suitable for profiling with `pprof` and stack traces from `SIGQUIT`.

### Release Build

A release build strips debug symbols and embeds the version string. Use this for binaries you distribute or deploy.

```bash
VERSION=$(git describe --tags --always)
go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o homelite .
```

`-trimpath` removes absolute paths from the binary, making builds reproducible. `-s -w` strip the symbol table and DWARF info. The version string is surfaced by `/api/status`.

### Cross-Compile

Go cross-compiles without a C toolchain because the project is pure Go. Set `GOOS` and `GOARCH`:

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o homelite-linux-amd64 .

# Windows amd64
GOOS=windows GOARCH=amd64 go build -o homelite.exe .

# macOS arm64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o homelite-darwin-arm64 .

# macOS amd64 (Intel)
GOOS=darwin GOARCH=amd64 go build -o homelite-darwin-amd64 .

# Raspberry Pi 4 (64-bit OS)
GOOS=linux GOARCH=arm64 go build -o homelite-rpi-arm64 .
```

> **Note:** If the build fails with `pattern X: no matching files found`, the embedded assets (`index.html`, `logo.ico`, `startup.mp3`) are missing from the repository. Run `git submodule update --init` if they live in a submodule, or restore them from a release tarball. The build will not succeed without them.

## Configuration

HomeLite persists configuration in the `settings` table of the SQLite database. Most settings can be changed at runtime through the admin panel; a few require a restart. Settings are written through `PUT /api/admin/settings` and take effect immediately unless marked otherwise.

### Settings Table

| Key                   | Default       | Meaning                                            | Restart? |
|-----------------------|---------------|----------------------------------------------------|----------|
| `server_display_name` | `HomeLite`    | Shown in the header and terminal banner            | No       |
| `port`                | `8080`        | HTTP listening port                                | Yes      |
| `max_upload_bytes`    | `5368709120`  | Per-upload size limit (5 GiB)                      | No       |
| `default_quota_bytes` | `53687091200` | Per-user default storage quota (50 GiB)            | No       |
| `session_hours`       | `72`          | Session lifetime in hours                          | No       |
| `registration_open`   | `false`       | Reserved; currently admin-only account creation    | No       |
| `theme`               | `system`      | `system` \| `light` \| `dark`                      | No       |

`default_quota_bytes` applies only to newly created users. Existing users keep their assigned quota; change per-user in the admin panel.

### Environment Variables

| Variable            | Purpose                       | Default   |
|---------------------|-------------------------------|-----------|
| `HOMELITE_DATA_DIR` | Override the data directory   | `./data`  |
| `HOMELITE_NO_ANIM`  | Skip the terminal splash      | unset     |

Environment variables take precedence over defaults but do not override settings written to the database. `HOMELITE_DATA_DIR` is read at startup and cannot be changed without a restart.

### Command-Line Flags

| Flag        | Effect                       |
|-------------|------------------------------|
| `--no-anim` | Skip the terminal splash     |

Flags are evaluated after environment variables. `--no-anim` and `HOMELITE_NO_ANIM=1` are equivalent.

### Changing the Port

Set `port` in the admin panel (Settings) and restart the process. Changing the value in the database without restarting has no effect — the listener is bound once at startup.

### Changing the Data Directory

Set `HOMELITE_DATA_DIR` before launching:

```bash
export HOMELITE_DATA_DIR=/var/lib/homelite
./homelite
```

Windows (PowerShell):

```powershell
$env:HOMELITE_DATA_DIR = "C:\ProgramData\HomeLite"
.\homelite.exe
```

Windows (Command Prompt):

```cmd
set HOMELITE_DATA_DIR=C:\ProgramData\HomeLite
homelite.exe
```

The directory is created on first run if it does not exist. Moving an existing data directory is supported — shut the server down first, then move the entire directory and update `HOMELITE_DATA_DIR`. Do not move the directory while the server is running; SQLite may have WAL pages in shared memory that will be lost.

## Data Directory Layout

```text
data/
├── homelite.db          SQLite database (metadata, users, sessions, settings)
├── homelite.db-wal      WAL (transient, SQLite-managed)
├── homelite.db-shm      Shared memory (transient, SQLite-managed)
├── files/
│   ├── 1/               Content for user id 1 (UUID-named files)
│   └── 2/               Content for user id 2
├── temp/                Temporary uploads (auto-cleaned on startup)
└── logs/                Reserved
```

### What to Back Up

- **Back up:** `homelite.db` and the entire `files/` tree. Treat them as a unit — the database references files by UUID, and restoring one without the other will produce broken references.
- **Do not back up:** `homelite.db-wal`, `homelite.db-shm`, and `temp/`. These are transient; they are recreated on startup or during normal operation.
- **Optional:** `logs/` if you enable application logging in the future. Currently reserved.

See [Backup and Restore](#backup-and-restore) for the recommended procedure.

## Using HomeLite

### First Run

1. Launch the binary. The terminal splash prints the LAN URL(s) the server is reachable at.
2. Open `http://localhost:8080` in a browser on the same machine.
3. The first-run setup page appears (only when no users exist). Choose an administrator username and password.
4. You are logged in and redirected to the dashboard.

The setup page disappears once the first admin exists. Subsequent users are created through the admin panel.

### Daily Use

- Drag files onto the dashboard to upload. Progress is shown per file. Multiple files upload sequentially through the upload semaphore.
- Use the breadcrumb at the top to navigate folders. Click a folder to enter it.
- Use the search bar to filter by filename across all of your folders. The filter is debounced — type and the list updates.
- Click a column header to sort by name, size, or upload date. Click again to reverse direction.
- Use the row action buttons to rename, move, or delete a file or folder. Folder delete is recursive — confirm carefully.
- The storage indicator at the bottom shows your usage against your quota.
- Click your username to open the profile modal — change password, view session info.

### Admin Tasks

Administrators see an additional "Admin" entry in the user menu:

- **Users:** Create, disable, enable, delete, reset password, change quota, change role.
- **Settings:** Edit the values in the settings table. The UI indicates which changes require a restart.

Disabled users cannot log in, but their files are retained. Deleting a user removes their files and metadata irreversibly.

### Profile

- Change your password. Changing it invalidates all of your other sessions after the response is sent.
- View your current quota usage and the breakdown by folder (top-level only).

### Planned Keyboard Shortcuts

The following shortcuts are planned (not all are wired in the current build):

| Shortcut | Action                       |
|----------|------------------------------|
| `/`      | Focus search                 |
| `u`      | Open upload picker           |
| `n`      | New folder                   |
| `Delete` | Delete selection             |
| `F2`     | Rename selection             |
| `?`      | Show shortcut help           |

## API Reference

All endpoints return JSON. Errors use the shape `{ "error": "code", "message": "human-readable text" }` with an appropriate HTTP status: `400`, `401`, `403`, `404`, `409`, `413`, `429`, `500`, or `507`.

All mutating requests (`POST`, `PUT`, `PATCH`, `DELETE`) on authenticated endpoints require the `X-CSRF-Token` header with the session's CSRF token. The token is returned by `/api/login` and `/api/me`.

### Public Endpoints

| Method | Path                       | Body                                  | Response |
|--------|----------------------------|---------------------------------------|----------|
| `GET`  | `/api/status`              | —                                     | `{ server_name, needs_setup, file_count, user_count, total_size, uptime, lan_ip, port, bytes_served, max_upload_bytes }` |
| `POST` | `/api/setup`               | `{ username, password }`              | `201 { ok }` |
| `POST` | `/api/login`               | `{ username, password }`              | `{ ok, csrf }` |
| `POST` | `/api/logout`              | —                                     | `{ ok }` |

`/api/setup` returns `409 Conflict` if any users already exist. `/api/login` returns `429 Too Many Requests` after 8 failed attempts within 15 minutes per username + IP.

### Authenticated Endpoints

Requires a valid session cookie. CSRF token required on all mutations.

| Method   | Path                              | Body / Query                                  | Response |
|----------|-----------------------------------|-----------------------------------------------|----------|
| `GET`    | `/api/me`                         | —                                             | `{ user, csrf }` |
| `POST`   | `/api/me/password`                | `{ old_password, new_password }`              | `{ ok }` |
| `GET`    | `/api/files`                      | `?parent_id=&q=&sort=&order=`                 | `{ files: [...], user }` |
| `POST`   | `/api/files/mkdir`                | `{ name, parent_id }`                         | `{ ok, id }` |
| `POST`   | `/api/files/upload`               | multipart: `parent_id`, `file`                | `{ ok, id, name, size, sha256 }` |
| `POST`   | `/api/files/:id/rename`           | `{ name }`                                    | `{ ok }` |
| `POST`   | `/api/files/:id/move`             | `{ parent_id }`                               | `{ ok }` |
| `DELETE` | `/api/files/:id`                  | —                                             | `{ ok }` |
| `GET`    | `/api/files/:id/download`         | —                                             | File stream with `Content-Disposition` |

`sort` accepts `name`, `size`, or `uploaded_at`. `order` accepts `asc` or `desc`. `parent_id` empty or omitted lists root. Ownership is verified on every endpoint; a request for another user's file ID returns `404`, not `403`, to avoid leaking existence.

### Admin Endpoints

Requires an authenticated administrator session.

| Method   | Path                          | Body                                                | Response |
|----------|-------------------------------|-----------------------------------------------------|----------|
| `GET`    | `/api/admin/users`            | —                                                   | `{ users: [...] }` |
| `POST`   | `/api/admin/users`            | `{ username, password, role, quota_bytes }`         | `{ ok }` |
| `PATCH`  | `/api/admin/users/:id`        | `{ disabled?, quota_bytes?, role?, new_password? }` | `{ ok }` |
| `DELETE` | `/api/admin/users/:id`        | —                                                   | `{ ok }` |
| `GET`    | `/api/admin/settings`         | —                                                   | `{ key: value, ... }` |
| `PUT`    | `/api/admin/settings`         | partial `{ key: value, ... }`                       | `{ ok, restart_required_for: [...] }` |

`DELETE /api/admin/users/:id` refuses to delete the last remaining admin. Promote another user first.

### Example: Upload a File

```bash
CSRF=$(curl -s -c cookies.txt -X POST http://localhost:8080/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"hunter2"}' | jq -r .csrf)

curl -b cookies.txt -X POST http://localhost:8080/api/files/upload \
  -H "X-CSRF-Token: $CSRF" \
  -F "parent_id=" \
  -F "file=@/path/to/photo.jpg"
```

### Example: List Files in a Folder

```bash
curl -b cookies.txt \
  "http://localhost:8080/api/files?parent_id=abc-123&sort=name&order=asc"
```

### Example: Download a File

```bash
curl -b cookies.txt -OJ \
  "http://localhost:8080/api/files/abc-123/download"
```

`-OJ` tells curl to use the server-supplied filename from `Content-Disposition`, which preserves Unicode names.

## Accessing From Other Devices

HomeLite is designed for LAN use. Once the server is running, any device on the same network can reach it.

### Find the LAN URL

The terminal splash prints every URL the server is reachable at. Look for the line that begins with `LAN:` — for example:

```text
LAN:  http://192.168.1.42:8080
```

On a phone or tablet connected to the same Wi-Fi, open that URL in the browser and log in. The dashboard is responsive and works in mobile Safari and Chrome.

### Windows Firewall

On Windows, the first run will usually prompt to allow network access. If you missed the prompt or run as a service, allow the port manually.

Windows (PowerShell, as Administrator):

```powershell
New-NetFirewallRule -DisplayName "HomeLite" -Direction Inbound `
  -Protocol TCP -LocalPort 8080 -Action Allow -Profile Private
```

> **Use the `Private` profile only.** Allowing HomeLite on the `Public` profile exposes it to untrusted networks (cafes, hotels, hotspots). If you carry the laptop between networks, prefer running HomeLite only when connected to a trusted network.

### Troubleshooting Checklist

1. **Both devices on the same LAN?** Some guest networks isolate clients. Verify both are on the same SSID, not a guest SSID.
2. **Server still running?** Check the terminal where you launched it. If it crashed, look at the last log line.
3. **Firewall allows the port?** See the command above. On macOS, the firewall prompt appears on first run.
4. **Correct IP?** The splash prints it. Do not use `localhost` or `127.0.0.1` from another device.
5. **Browser accepts HTTP?** HomeLite serves plain HTTP on the LAN. Browsers may warn on form submissions — that is expected.
6. **Antivirus or VPN interfering?** Some consumer VPNs block LAN traffic. Disable to test.
7. **AP isolation enabled on the router?** Some access points enable client isolation by default. Disable it for your trusted SSID.

## Remote Access

> **Do not expose HomeLite directly to the internet over HTTP.** Session cookies and login credentials travel in plaintext. Anyone on the path between your device and the server can capture them. The `Secure` cookie flag is only set under TLS; a plaintext deployment does not get it.

### Option 1 — VPN (Recommended)

Run a VPN on your home network (WireGuard, Tailscale, OpenVPN). Connect from your remote device, then access HomeLite by its LAN IP. This keeps the server off the public internet entirely. Tailscale in particular requires no port forwarding and works through NAT.

### Option 2 — Cloudflare Tunnel

Cloudflare Tunnel exposes a local service through Cloudflare's network without opening inbound ports. It terminates TLS at Cloudflare's edge.

```bash
# Install cloudflared, then:
cloudflared tunnel create homelite
cloudflared tunnel route dns homelite files.example.com
cloudflared tunnel run --url http://localhost:8080 homelite
```

Configure Cloudflare Access in front of the tunnel for an additional authentication layer. Note that Cloudflare Access will strip the `X-Forwarded-Proto` header in some configurations — verify that the `Secure` cookie flag is being set by inspecting cookies in your browser's devtools.

### What Not To Do

- **Do not port-forward 8080 to the public internet.** The session cookie will be sniffable. Even with HTTPS, an exposed login form is a target for credential stuffing.
- **Do not rely on the `Secure` cookie flag alone.** It is only set when `X-Forwarded-Proto: https` is present or TLS is active. A misconfigured reverse proxy that omits this header will downgrade cookies.
- **Do not place HomeLite on a public IP without TLS.** Use a reverse proxy (Caddy, nginx, Traefik) with a real certificate. Caddy in particular can issue certificates automatically.
- **Do not run HomeLite as root to bind port 80 or 443.** Use port forwarding (`iptables`, `netsh`, or a reverse proxy) instead. Root-privileged Go binaries are a needless attack surface.

## Backup and Restore

A consistent backup requires both `homelite.db` and the `files/` tree. Back up one without the other and you will have dangling references in either direction — files with no metadata, or metadata pointing at missing files.

### Back Up the Database

Use SQLite's online backup API via the `sqlite3` CLI. This works while the server is running and produces a consistent snapshot.

```bash
sqlite3 data/homelite.db ".backup data/homelite.db.bak"
```

If you do not have the `sqlite3` CLI installed, copy the file with the server stopped:

```bash
# Stop the server first, then:
cp data/homelite.db data/homelite.db.bak
```

Copying the database while the server is running risks capturing a half-written WAL state. Prefer the `.backup` command.

### Back Up the Files

`rsync` is the safe choice — it preserves permissions, handles partial transfers, and is resumable.

```bash
rsync -a --delete data/files/ /backup/homelite/files/
```

For Windows, `robocopy` is the equivalent:

```cmd
robocopy C:\ProgramData\HomeLite\files D:\Backup\homelite\files /MIR
```

### Automated Backup

A minimal cron entry that takes a consistent snapshot of both database and files:

```bash
# /etc/cron.d/homelite-backup
0 3 * * *  homelite  sqlite3 /var/lib/homelite/data/homelite.db ".backup /backup/homelite/$(date +\%F).db" && rsync -a --delete /var/lib/homelite/data/files/ /backup/homelite/files/
15 3 * * * homelite  find /backup/homelite -name '*.db' -mtime +14 -delete
```

This takes a nightly backup at 03:00 and removes database snapshots older than two weeks. Tune retention to your storage budget.

### Restore

1. Stop the server.
2. Restore the database: `cp /backup/homelite.db.bak data/homelite.db`.
3. Restore the files: `rsync -a /backup/homelite/files/ data/files/`.
4. Restart the server.

If the database is newer than the files (for example, you restored a recent database backup but an older files backup), uploads that happened between the two snapshots will be listed but will 404 on download. The orphan cleanup does not remove these because the database references them; you must reconcile manually or restore to a matching pair.

### Integrity Check

After restoring the database, run an integrity check:

```bash
sqlite3 data/homelite.db "PRAGMA integrity_check;"
```

Expect `ok`. Anything else indicates corruption — restore from an earlier backup. A `foreign_key_check` is also useful:

```bash
sqlite3 data/homelite.db "PRAGMA foreign_key_check;"
```

This returns rows for any orphaned child records. An empty result is healthy.

## Security Model

### What Is Implemented

- Argon2id password hashing (memory 64 MiB, time 2, threads 4, key 32 B).
- Server-side sessions in SQLite with configurable lifetime. Sessions are not JWTs; they are opaque tokens looked up in the database.
- CSRF tokens, constant-time compared, required on every mutating request. Tokens rotate per session.
- Login throttling: 8 attempts per 15 minutes per username + IP. Counter is in-memory.
- `HttpOnly`, `SameSite=Lax` cookies. `Secure` flag set under TLS or `X-Forwarded-Proto: https`.
- Per-user file isolation. Ownership is checked on every read and write, not just at listing time.
- Path traversal prevented. Clients reference file IDs only; physical paths are UUIDs under `data/files/<user_id>/`.
- Filenames sanitized: no path separators, no null bytes, no `..`. The original filename is stored as metadata only.
- MIME type inferred from extension, never from client-supplied `Content-Type`. This prevents malicious MIME sniffing.
- Parameterized SQL throughout. No string interpolation into queries.
- SHA-256 checksum computed during streaming upload. Stored alongside file metadata.
- Safe HTTP response headers: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`.
- Session fixation prevention — new token on every login.
- Password change invalidates all of the user's existing sessions, including the one used to make the change after the response is sent.

### What Is Not Implemented — Honest Limitations

> **No end-to-end encryption.** Files are plaintext on the host disk. Anyone with read access to `data/files/` can read your files. **Recommend full-disk encryption** (BitLocker on Windows, FileVault on macOS, LUKS on Linux) on the host machine. HomeLite does not encrypt files at rest.

- **No resumable uploads.** An interrupted upload starts over from byte zero. The temp file is removed on the next startup's orphan cleanup.
- **No 2FA / TOTP.** Authentication is password-only. There is no second factor.
- **No antivirus or content scanning.** HomeLite does not inspect file contents. Run a host-level scanner if you accept uploads from untrusted users.
- **Login throttling is in-memory and per-process.** A restart clears the counters. A multi-process deployment would share no throttling state.
- **Browser autoplay policy blocks `startup.mp3` on first visit** until the user interacts with the page. This is expected browser behavior, not a bug. Subsequent visits play the chime normally.
- **Designed for LAN use.** See [Remote Access](#remote-access) before exposing the server beyond your network.
- **Windows console ANSI colors require Windows 10 or later.** The binary enables `ENABLE_VIRTUAL_TERMINAL_PROCESSING` at startup; on Windows 8 or earlier, the splash will display without colors.
- **No audit log.** User actions are not recorded in a tamper-evident log. If you need this, run HomeLite behind a reverse proxy that logs requests.

### Threat Model

HomeLite defends against: unauthorized file access by other (non-admin) users, credential stuffing via throttling, CSRF, session fixation, path traversal, SQL injection, MIME confusion, and login replay via session token theft on the same network.

HomeLite does not defend against: a malicious admin (they can read all metadata and reset passwords), a host compromise (the attacker has the database and the files), network attackers when serving over plain HTTP off-LAN, or a user with physical access to the host disk. For those threats, use full-disk encryption, a VPN, and physical access controls respectively.

### Deployment Recommendations

- Run as a dedicated non-root user with read-write access only to the data directory.
- Place the data directory on an encrypted volume.
- Run behind a reverse proxy with TLS if accessed off-LAN.
- Back up the database and `files/` tree on a schedule. See [Backup and Restore](#backup-and-restore).
- Keep the binary updated. Subscribe to release notifications.
- Use a long, random admin password. Store it in a password manager.
- Create a second admin account as a recovery path in case the primary admin password is lost.

## Performance and Resource Limits

### Idle Memory

Idle RAM usage is dominated by Go's runtime and the open SQLite handle. Expect 30–60 MiB resident set on a 64-bit Linux build with no active requests. The embedded frontend is served from memory but is small (tens of kilobytes). On Windows and macOS, idle usage is similar.

### Streaming Behavior

Uploads and downloads are streamed. Files are never fully buffered in RAM. Memory pressure during a transfer is bounded by the buffer size used by `io.Copy` (typically 32 KiB) plus multipart parsing overhead. A 5 GiB upload and a 5 MiB upload have similar peak memory usage. SHA-256 is computed during the stream, so the checksum is free at the I/O level.

### Concurrency Limits

| Resource                       | Limit                  |
|--------------------------------|------------------------|
| Simultaneous uploads           | 4 (semaphore)          |
| SQLite write connections       | 1 (serialized by WAL)  |
| SQLite read connections        | Pool-tuned internally  |
| Per-upload size                | `max_upload_bytes` (default 5 GiB) |
| Per-user storage               | `default_quota_bytes` (default 50 GiB, per-user overridable) |
| Login attempts                 | 8 per 15 min per username + IP |

### Tuning

- **Raise `max_upload_bytes`** in the admin panel if you regularly upload larger files. The limit is enforced before the body is read past that size, so a client that lies about `Content-Length` will still be rejected when the byte count crosses the threshold.
- **Raise `default_quota_bytes`** to give new users more space. Existing users keep their assigned quota; change per-user in the admin panel.
- **Lower `session_hours`** for a stricter security posture. Short sessions reduce the impact of a stolen cookie.
- **Do not raise the upload concurrency above 4** without testing disk throughput. Spinning disks degrade badly under random writes; SSDs tolerate more but SQLite's single-writer serialization is the real bottleneck.
- **SQLite pool size** is fixed internally. WAL mode allows concurrent readers and one writer; tuning the pool will not increase write throughput. If you need more write throughput than SQLite provides, HomeLite is the wrong tool.

### What to Monitor

- `data/homelite.db-wal` size. If it grows unbounded, the WAL checkpoint is not keeping up. Ensure the server is being shut down gracefully (it runs `PRAGMA wal_checkpoint(TRUNCATE)` on shutdown).
- Disk space on the volume holding `data/`. Quotas prevent a single user from filling the disk, but they do not prevent the admin from under-provisioning.
- Memory usage under load. If it climbs unexpectedly, an upload may be getting buffered; check that the client is using chunked transfer or a known `Content-Length`.

## Project Structure

```text
homelite/
├── main.go                 Entry point: flags, splash, server startup, shutdown
├── server.go               HTTP server, router, middleware chain
├── handlers.go             Request handlers for /api/*
├── auth.go                 Argon2id, sessions, CSRF, login throttling
├── store.go                SQLite layer: schema, migrations, queries
├── files.go                File system layer: upload, download, move, delete
├── settings.go             Settings table accessors
├── admin.go                Admin-only handlers
├── middleware.go           Auth, CSRF, logging, security headers
├── embed.go                //go:embed directives for assets
├── static/
│   ├── index.html          Embedded frontend (HTML + CSS + vanilla JS)
│   ├── logo.ico            Embedded favicon
│   └── startup.mp3         Embedded startup chime
├── go.mod                  Module definition
├── go.sum                  Dependency checksums
├── LICENSE                 MIT license
└── README.md               This file
```

## Troubleshooting

| Symptom                                                | Cause                                                                 | Fix                                                                                       |
|--------------------------------------------------------|-----------------------------------------------------------------------|-------------------------------------------------------------------------------------------|
| `bind: address already in use` on startup              | Another process owns the port                                         | Change `port` in the admin panel, or stop the other process.                              |
| Other devices cannot reach the server on the LAN       | Firewall blocks the port                                              | Allow inbound TCP on the port (see [Accessing From Other Devices](#accessing-from-other-devices)). |
| `permission denied` writing to the data directory      | The runtime user lacks write access                                   | `chown` the data directory to the runtime user, or set `HOMELITE_DATA_DIR` to a writable path. |
| `pattern X: no matching files found` during build      | Embedded assets (`index.html`, `logo.ico`, `startup.mp3`) are missing | Restore the files from the repo or a release tarball.                                     |
| `database is locked` (HTTP 500)                        | Another process has the database open, or WAL checkpoint stalled       | Ensure only one HomeLite process uses the data directory. Restart if persistent.          |
| Upload stalls at a percentage and never completes      | Network drop or client closed the connection mid-stream               | Cancel and retry. There is no resumable upload.                                           |
| Favicon does not update after replacement              | Browser cache                                                         | Hard refresh (Ctrl+Shift+R / Cmd+Shift+R), or clear the favicon cache.                    |
| Startup sound does not play on first visit             | Browser autoplay policy                                               | Click anywhere on the page; subsequent loads will play it.                                |
| Terminal splash shows no colors on Windows             | Old Windows console                                                   | Use Windows 10 or later. The binary enables VT processing on startup.                     |
| `407 Insufficient Storage` on upload                   | User quota exceeded                                                   | Ask an admin to raise the user's `quota_bytes`, or delete files first.                    |
| `429 Too Many Requests` on login                       | Login throttling                                                      | Wait 15 minutes, or restart the server (clears in-memory counters).                       |
| `413 Payload Too Large` on upload                      | Single upload exceeds `max_upload_bytes`                              | Raise `max_upload_bytes` in the admin panel, or split the file.                           |
| Database grows but `files/` does not                   | Orphaned metadata from interrupted deletes                            | Run `PRAGMA foreign_key_check;` and reconcile.                                            |

## FAQ

| # | Question | Answer |
|---|----------|--------|
| 1 | Can I access HomeLite over the internet? | Not directly. Use a VPN or a TLS-terminating reverse proxy. See [Remote Access](#remote-access). |
| 2 | Is my data encrypted? | Files are plaintext on the host disk. Use full-disk encryption (BitLocker / FileVault / LUKS). Transit encryption requires TLS via a reverse proxy. |
| 3 | Can I run it on a Raspberry Pi? | Yes. Cross-compile with `GOOS=linux GOARCH=arm64`. Tested on Pi 3 and later with a 64-bit OS. Idle memory fits comfortably in 1 GiB. |
| 4 | Can I move the data directory? | Yes. Stop the server, move the entire directory, set `HOMELITE_DATA_DIR`, restart. Do not move it while the server is running. |
| 5 | Can I have multiple admin users? | Yes. Create users in the admin panel and assign the admin role. Any admin can manage other users. Recommended as a recovery path. |
| 6 | What happens on Ctrl-C during an upload? | The server starts graceful shutdown. The in-flight upload is cancelled and its temp file is removed on the next startup's orphan cleanup. There is no resume. |
| 7 | Does it work on Windows 10? | Yes. ANSI color support requires Windows 10 or later. Older versions will run but without colored splash. |
| 8 | Does it need Docker? | No. HomeLite is a single binary. There is no Docker image and none is planned. |
| 9 | Where are passwords stored? | In `homelite.db`, hashed with Argon2id. They are never stored in plaintext or written to logs. |
| 10 | How do I reset a forgotten admin password? | If another admin exists, use the admin panel. If you are the only admin and have lost the password, there is no built-in recovery — restore `homelite.db` from a backup taken when the password was known. Prevent this by always having at least two admin accounts. |
| 11 | Can I change the port? | Yes, in the admin panel under Settings. A restart is required for the change to take effect. |
| 12 | Is there a mobile app? | No. The web dashboard is responsive and works in mobile browsers. There are no plans for a native app. |
| 13 | What if my laptop sleeps? | The server stops responding while asleep. Clients will see connection errors. Resuming the laptop restores service. Long uploads in progress will fail and need to be restarted. |
| 14 | How do I update to a new version? | Stop the server, replace the binary, restart. Schema migrations run automatically on startup. Keep a backup of `homelite.db` before updating. |
| 15 | Can I disable the startup sound? | Yes. Set `HOMELITE_NO_ANIM=1` to skip the splash and chime, or use the `--no-anim` flag. The browser will also not autoplay audio until you interact with the page. |

## License

HomeLite is licensed under the [MIT License](LICENSE).
```
