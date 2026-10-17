# HomeLite

**Your files, your hardware, your private cloud.**

HomeLite is a self-hosted file storage server written in Go that compiles into a single standalone executable. Run it on a Windows PC, Linux machine, Mac, or home server, then access your files through a web dashboard from other devices on your local network. HomeLite stores file contents on your hardware and uses SQLite for metadata, accounts, sessions, and settings. It requires no Docker, Node.js, Python, external database server, frontend build system, or third-party runtime service.

## Table of Contents

- [Design Principles](#design-principles)
- [Features](#features)
- [Screenshots](#screenshots)
- [How It Works](#how-it-works)
- [Quick Start](#quick-start)
- [Building From Source](#building-from-source)
- [Configuration](#configuration)
- [Data Directory Layout](#data-directory-layout)
- [Using HomeLite](#using-homelite)
  - [First Run](#first-run)
  - [Daily Use](#daily-use)
  - [Administration](#administration)
  - [Profile and Passwords](#profile-and-passwords)
  - [Keyboard Shortcuts](#keyboard-shortcuts)
- [API Reference](#api-reference)
  - [Public Endpoints](#public-endpoints)
  - [Authentication and Profile](#authentication-and-profile)
  - [File Operations](#file-operations)
  - [Administration API](#administration-api)
  - [Errors](#errors)
- [Accessing From Other Devices](#accessing-from-other-devices)
- [Remote Access](#remote-access)
- [Backup and Restore](#backup-and-restore)
- [Security Model](#security-model)
- [Performance and Resource Limits](#performance-and-resource-limits)
- [Project Structure](#project-structure)
- [Troubleshooting](#troubleshooting)
- [FAQ](#faq)
- [License](#license)

---

## Design Principles

- **Single-binary simplicity:** Build HomeLite into one executable containing the application, frontend, favicon, and startup sound.
- **Data sovereignty:** Store files and application metadata on hardware you control. No telemetry or mandatory third-party storage service.
- **Minimal runtime dependencies:** Use Go's standard HTTP library, a pure-Go SQLite driver, and an embedded HTML/CSS/JavaScript frontend.
- **Server-side security:** Authenticate protected requests, verify file ownership, enforce quotas, and use parameterized SQL queries.
- **Honest security posture:** Document the difference between application-level access controls and protections that require HTTPS, disk encryption, or a VPN.

## Features

### Storage

- Upload files through a file picker or drag-and-drop.
- Stream uploads directly to disk instead of buffering entire files in memory.
- Display per-file upload progress.
- Process multiple selected files sequentially.
- Download files with their original filenames and extensions.
- Support Unicode filenames in download responses.
- Create folders and nested directories.
- Navigate directories using breadcrumbs.
- Rename, move, and delete files.
- Rename, move, and recursively delete folders.
- Search filenames across folders using a debounced live filter.
- Sort files by name, size, or upload date, in ascending or descending order.
- Enforce per-user storage quotas when uploading.
- Calculate SHA-256 checksums while receiving uploads.
- Keep physical storage filenames separate from user-visible filenames.

### Authentication

- Create the initial administrator through a first-run setup page.
- Hash passwords with Argon2id.
- Store server-side sessions in SQLite.
- Configure session lifetime, defaulting to 72 hours.
- Generate a CSRF token for each session.
- Require the `X-CSRF-Token` header on mutating requests.
- Compare CSRF tokens in constant time.
- Throttle login attempts to eight attempts per 15 minutes per username and IP address.
- Use `HttpOnly` and `SameSite=Lax` session cookies.
- Set the cookie's `Secure` attribute when TLS is active or `X-Forwarded-Proto: https` is detected.
- Generate a new session token on every successful login.
- Invalidate all sessions belonging to a user when their password changes.
- Create, disable, enable, and delete user accounts through the administrator interface.
- Reset user passwords through administrator controls.
- Configure an individual storage quota for each user.

### Dashboard

- Responsive layout for phone, tablet, and desktop screens.
- Dark and light themes.
- System theme detection through `prefers-color-scheme`.
- Manual theme selection.
- File-type icons with category-specific colors.
- Live filename search.
- Sortable file listings.
- Breadcrumb navigation.
- Storage usage indicator and progress bar.
- Success, error, and informational toast notifications.
- Modals for rename, move, delete, administration, and profile operations.
- Loading indicators and empty states.
- Administrative user management.
- Embedded assets that work without a CDN or frontend build step.

### Operations

- One standalone executable per target platform.
- Go 1.22 or later.
- HTTP handling through `net/http`.
- SQLite through `modernc.org/sqlite`, without CGO.
- Automatic schema creation and migration at startup.
- SQLite WAL mode, foreign-key enforcement, and a 10-second busy timeout.
- Graceful shutdown on supported termination signals.
- WAL checkpoint during graceful shutdown.
- Cleanup of orphaned temporary uploads at startup.
- Bounded upload concurrency: at most four simultaneous uploads.
- Streaming downloads using `io.Copy`.
- Terminal startup animation with LAN URL information.
- Embedded `/favicon.ico` and `/startup.mp3` resources.

## Screenshots

> **Maintainer note:** Save actual application screenshots in `docs/screenshots/` using the filenames below. Capture screenshots from a running HomeLite instance, remove any private information, and commit the PNG files. Replace or remove any screenshot reference that does not correspond to an implemented interface. These placeholders are not screenshots of the application.

<!--
Screenshot contribution instructions:

1. Create docs/screenshots/ if it does not exist.
2. Capture the actual HomeLite UI at the recommended viewport.
3. Save each image using the exact filename referenced below.
4. Use PNG format and avoid including passwords, API tokens, or private files.
5. Review the screenshots after UI changes and update them when necessary.

Expected files:
- docs/screenshots/dashboard.png
- docs/screenshots/dashboard-dark.png
- docs/screenshots/mobile.png
- docs/screenshots/upload-progress.png
- docs/screenshots/admin-users.png
- docs/screenshots/settings.png
-->

### File dashboard

![HomeLite file dashboard](docs/screenshots/dashboard.png)

### Dark theme

![HomeLite dashboard in dark theme](docs/screenshots/dashboard-dark.png)

### Mobile layout

![HomeLite dashboard on a mobile device](docs/screenshots/mobile.png)

### Upload progress

![HomeLite upload progress indicators](docs/screenshots/upload-progress.png)

### Administrator panel

![HomeLite user administration panel](docs/screenshots/admin-users.png)

### Settings

![HomeLite configuration settings](docs/screenshots/settings.png)

## How It Works

HomeLite runs an HTTP server inside a Go executable. The browser communicates with its HTTP API and serves the embedded dashboard. SQLite stores account records, sessions, configuration, and file metadata. File contents live separately on the host filesystem, organized by user ID and stored under generated UUID filenames.

```text
+--------------------+
| Browser            |
| Phone / PC / Tablet|
+--------------------+
          |
          | HTTP requests and file streams
          v
+-----------------------------+
| HomeLite Go binary          |
|                             |
| net/http                    |
| Authentication and CSRF     |
| Authorization and quotas    |
| File and folder operations  |
| Embedded web dashboard     |
+-----------------------------+
          |
          +-------------------------+
          |                         |
          v                         v
+---------------------+   +----------------------+
| SQLite              |   | Filesystem           |
|                     |   |                      |
| Users and sessions  |   | data/files/<user_id>/|
| Metadata and quotas |   | UUID-named files     |
| Settings            |   | Temporary uploads    |
+---------------------+   +----------------------+


HomeLite does not require an external database server. The executable, database, and file storage can all reside on the same computer.
Important: The dashboard and API use HTTP by default. HTTP is suitable for a trusted LAN deployment, but credentials and session cookies are not encrypted in transit. Use a VPN or properly configured HTTPS reverse proxy when connecting over untrusted networks.
Quick Start
You can start with a prebuilt binary or compile HomeLite from source.
Option A: Prebuilt binary
Download the appropriate executable from the project's GitHub Releases page:
- Windows: homelite.exe
- Linux: homelite
- macOS: homelite
Replace OWNER/REPO in the following links with the actual GitHub repository coordinates before publishing this README.
- Browse releases
- Browse source code
Do not download executable files from untrusted mirrors.
Windows
Open Command Prompt in the directory containing homelite.exe:
homelite.exe


HomeLite uses ./data as its default data directory. The directory is resolved relative to the process's working directory unless HOMELITE_DATA_DIR overrides it.
Linux
Download the Linux executable, make it executable, and run it:
chmod +x ./homelite
./homelite


macOS
Download the executable built for the appropriate macOS architecture:
chmod +x ./homelite
./homelite


If macOS blocks an executable downloaded from the internet, verify its origin and any available release checksums before deciding whether to allow it to run.
Open the dashboard
After startup, open the local address in a browser:
http://localhost:8080


If the startup banner displays a LAN address, another device on the same network can use that address instead.
Create the administrator
On the first run, HomeLite presents an administrator setup page.
1. Choose an administrator username.
2. Set a strong, unique password.
3. Submit the setup form.
4. Sign in with the new account.
5. Create additional accounts through the administrator panel if needed.
The initial setup endpoint should only create the first administrator when no users exist. Once setup is complete, subsequent requests to create the initial administrator must be rejected.
Do not expose the first-run setup page to an untrusted network. Complete setup before configuring remote access.
Option B: Build from source
Install Go 1.22 or later, clone the repository, fetch the declared Go modules, and build the executable.
git clone https://github.com/OWNER/REPO.git
cd REPO
go mod download
go build -o homelite .
./homelite


On Windows, build the executable with:
go build -o homelite.exe .
homelite.exe


The commands above assume the Go main package is at the repository root. If the actual entry point is in a subdirectory, adjust the build target to that package.
For reproducible release builds, see Building From Source.
Building From Source
Requirements
Requirement	Version or detail
Go	1.22 or later
SQLite	Bundled through the pure-Go modernc.org/sqlite driver
C compiler	Not required
MSVC	Not required
Node.js	Not required
Python	Not required
Docker	Not required
Frontend toolchain	Not required
External database server	Not required
You need internet access when downloading Go modules for the first time. Once the dependencies are cached, builds can generally run offline.
1. Install Go
Install a supported Go release from the official Go downloads page.
Verify the installation:
go version


The output should report Go 1.22 or later.
On Windows Command Prompt:
go version


If the command is not found, ensure the Go installation's bin directory is available through PATH.
2. Clone the repository
git clone https://github.com/OWNER/REPO.git
cd REPO


Alternatively, download and extract a source archive from the project's release page.
3. Fetch dependencies
go mod download
go mod verify


HomeLite's application dependencies include:
- modernc.org/sqlite for pure-Go SQLite.
- golang.org/x/crypto/argon2 for Argon2id password hashing.
Other packages should only be added when required by the implementation. The HTTP server and frontend do not require an external framework.
4. Build a development executable
For Linux and macOS:
go build -o homelite .


For Windows:
go build -o homelite.exe .


Run the executable and verify that the first-run setup page appears:
./homelite


5. Build an optimized release
For a smaller, stripped Linux or macOS executable:
go build -trimpath -ldflags="-s -w" -o homelite .


For Windows:
go build -trimpath -ldflags="-s -w" -o homelite.exe .


The -trimpath option removes local filesystem paths from build metadata where supported. The -s -w linker flags strip symbol-table and debug information.
These flags reduce the binary's size but do not guarantee a particular final size. The result depends on the Go version, target platform, dependencies, and embedded assets.
Release verification: Run the built executable and test setup, authentication, upload, download, folder operations, quotas, administrator functions, and restart persistence before distributing it.
6. Cross-compile
Go supports cross-compilation by selecting GOOS and GOARCH. Since HomeLite uses a pure-Go SQLite driver, it does not require a C toolchain for the targets supported by its dependencies.
Windows 64-bit from Linux or macOS
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o homelite.exe .


Linux 64-bit from Windows, Linux, or macOS
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o homelite .


Linux ARM64
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o homelite .


macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o homelite .


macOS Intel
GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o homelite .


These commands assume the application supports the specified targets and that the selected Go version and dependencies compile for them.
A successful cross-build does not prove runtime compatibility. Test each release on its target operating system and architecture.
7. Run tests and static checks
If tests are present in the repository, run:
go test ./...


Run the Go static analyzer:
go vet ./...


Run the race detector on supported development platforms:
go test -race ./...


The race detector increases resource use and is primarily a development tool. It should not be treated as a substitute for testing concurrent uploads, SQLite transactions, or authorization boundaries.
8. Embedded frontend assets
HomeLite embeds its web assets in the Go binary with //go:embed.
The intended embedded resources are:
- index.html
- logo.ico
- startup.mp3
The files must exist at the paths specified by the actual go:embed directives. Go validates embedded paths during compilation.
Changing an embedded asset requires rebuilding the executable. Editing a separate copy of index.html after compilation does not change the frontend served by the existing binary.
Configuration
HomeLite stores application settings in the SQLite settings table. The database is the persistent source of truth for these settings.
Application settings
Key	Default	Description
server_display_name	HomeLite	Name displayed in the header and terminal banner.
port	8080	HTTP listening port. Changing it requires a restart.
max_upload_bytes	5368709120	Maximum size of one upload: 5 GiB.
default_quota_bytes	53687091200	Default quota for newly created users: 50 GiB.
session_hours	72	Session lifetime in hours.
registration_open	false	Reserved setting. Registration remains administrator-only.
theme	system	Default theme: system, light, or dark.
The byte limits use binary units:
- 1 GiB = 1,073,741,824 bytes.
- 5 GiB = 5,368,709,120 bytes.
- 50 GiB = 53,687,091,200 bytes.
The default quota is assigned to newly created users. Changing the default should not be assumed to change existing users' quotas; adjust individual quotas through administration where necessary.
Environment variables
Variable	Default	Purpose
HOMELITE_DATA_DIR	./data	Overrides the data directory.
HOMELITE_NO_ANIM	Unset	Skips the terminal startup animation when enabled.
The precise values recognized by HOMELITE_NO_ANIM depend on the implementation. Check the startup configuration code if you need to automate this setting.
Windows: change the data directory
In Command Prompt:
set HOMELITE_DATA_DIR=D:\HomeLiteData
homelite.exe


This applies to the current Command Prompt session.
In PowerShell:
$env:HOMELITE_DATA_DIR = "D:\HomeLiteData"
.\homelite.exe


The directory should be on a disk with sufficient free space and appropriate access permissions.
Linux and macOS: change the data directory
export HOMELITE_DATA_DIR="$HOME/homelite-data"
./homelite


To use a different directory for a single invocation:
HOMELITE_DATA_DIR=/srv/homelite ./homelite


Changing the data directory does not move existing data. Stop HomeLite, migrate the database and files as described in Backup and Restore, then start the application with the new path.
Command-line flags
Flag	Description
--no-anim	Skip the terminal startup animation.
Example:
./homelite --no-anim


Windows:
homelite.exe --no-anim


The environment variable and command-line flag both provide ways to suppress the terminal splash. The exact precedence between them should be verified against the implementation if both are set.
Change the listening port
The default port is 8080.
To change it:
1. Sign in as an administrator.
2. Open the administration settings panel.
3. Change the port setting.
4. Save the settings.
5. Restart HomeLite.
6. Open the new address in your browser.
For example, if you select port 9090, the local URL becomes:
http://localhost:9090


The administrator settings API reports settings that require a restart through restart_required_for.
If the configured port is already in use, the server cannot bind to it. Select another port or stop the conflicting process.
Change the data directory
The data directory can be overridden with HOMELITE_DATA_DIR.
A safe migration procedure is:
1. Stop HomeLite.
2. Back up the original database and files.
3. Copy the complete persistent data directory to the destination.
4. Verify that the destination contains the database and files/ directory.
5. Set HOMELITE_DATA_DIR to the destination.
6. Start HomeLite.
7. Sign in and verify that users and files are present.
8. Keep the original backup until the migration is verified.
Do not point two HomeLite processes at the same data directory unless the implementation explicitly supports multi-process operation. SQLite coordination alone does not establish that application-level filesystem operations are safe across multiple instances.
Data Directory Layout
HomeLite keeps the SQLite database and stored file contents under one data directory.
data/
├── homelite.db
├── homelite.db-wal
├── homelite.db-shm
├── files/
│   ├── 1/
│   │   ├── <uuid>
│   │   └── <uuid>
│   └── 2/
│       └── <uuid>
├── temp/
│   └── <temporary-upload-files>
└── logs/


Path	Purpose	Back up?
homelite.db	Users, sessions, settings, file metadata, and related records.	Yes
homelite.db-wal	SQLite write-ahead log. May contain committed database changes not yet checkpointed into the main file.	Include when making a raw, consistent copy of a live database; otherwise use SQLite's backup mechanism.
homelite.db-shm	SQLite shared-memory coordination file.	Normally no; SQLite can recreate it.
files/	Stored file contents organized by user ID.	Yes
temp/	Temporary upload data.	Usually no; interrupted uploads are not guaranteed to be recoverable.
logs/	Reserved directory.	Optional; it is not required for restoring the core file store.
Database metadata and physical files
The database maps a user's visible filename and directory structure to generated physical storage names.
A file displayed as report.pdf may be stored under a UUID-like name rather than report.pdf on disk. This separation helps prevent client-controlled filenames from becoming filesystem paths.
Do not rename, move, or delete individual files inside files/ manually while HomeLite is running. Doing so can break the relationship between database records and stored content.
What must be backed up
For a complete backup, preserve:
- The SQLite database through a consistent backup.
- The entire files/ directory.
- The relationship between the database and its stored files.
- Any external deployment configuration required to start the server.
The database and filesystem form one logical dataset. A database-only backup cannot restore file contents that are missing from the filesystem. A filesystem-only backup cannot reconstruct account records, folder metadata, or ownership mappings.
See Backup and Restore for the recommended procedure.
Using HomeLite
First Run
The first run initializes the database, applies schema migrations, prepares storage directories, and starts the HTTP server.
Follow these steps:
1. Run the executable from a directory where it has permission to create its data directory.
2. Read the startup output for the local URL and any detected LAN URL.
3. Open the URL in a browser.
4. Complete the administrator setup form.
5. Sign in.
6. Open the administrator panel.
7. Create accounts for other users.
8. Set appropriate storage quotas.
9. Upload a small test file.
10. Download the file and verify that its contents match the original.
The setup process should only be available while the database contains no users. Do not manually remove user records from SQLite to regain access to setup.
Daily Use
Upload files
1. Navigate to the destination folder.
2. Drag files into the upload area or open the file picker.
3. Watch the progress indicator for each file.
4. Wait for each upload to finish before closing the page or navigating away.
HomeLite processes multiple selected files sequentially. At most four uploads can be active at the server level, subject to the implementation's upload semaphore.
A browser may terminate a request if the page closes, the connection drops, or the device sleeps. Interrupted uploads are not resumable.
Create folders
Use the folder-creation control, enter a folder name, and confirm.
Folders can be nested. Use breadcrumbs to return to parent directories.
Rename and move items
Select the item's action menu, choose rename or move, and confirm the operation.
The server validates ownership and destination permissions. Clients should supply file IDs and logical folder IDs, not physical filesystem paths.
Download files
Select a file and use its download action. The server streams the file to the browser and supplies a Content-Disposition header intended to preserve the original filename, including Unicode characters.
The browser may choose where to save the downloaded file according to its own settings.
Delete items
Choose delete and confirm the operation.
Deleting a folder recursively removes its contents. This operation is destructive; verify the selected folder before confirming.
Search and sorting
Use the search field to filter files by name across folders. The search is debounced to avoid issuing a request for every keystroke.
Sort listings by:
- Name
- Size
- Upload date
Choose ascending or descending order as needed.
The search feature is filename-based, not full-text indexing of file contents.
Administration
Administrator accounts can manage other users through the admin panel.
Available administrative actions include:
- Create a user.
- Set a user's role.
- Set a user's storage quota.
- Disable a user.
- Re-enable a disabled user.
- Reset a user's password.
- Delete a user.
- View and change supported application settings.
A disabled account should not be able to continue making authenticated requests merely because it has an existing session. Protected requests must validate the account's current authorization state.
Deleting a user is a potentially destructive operation. Verify how the implementation handles the user's files and metadata before using deletion as a storage-cleanup method. Do not assume that account deletion always means permanent erasure of every related artifact unless this behavior is verified in the source.
Storage quotas
Each user has an individual quota. New accounts inherit the configured default_quota_bytes value unless a different quota is supplied during creation.
The upload handler must check quota availability before accepting a file and account for concurrent uploads. Checking quota only in the browser is insufficient.
An upload that exceeds the per-upload limit should return HTTP 413. An upload that cannot fit within the user's quota should return HTTP 507.
The quota limits apply to HomeLite's managed file storage. They do not reserve physical disk space or protect the operating system from running out of disk space.
Profile and Passwords
The profile interface allows users to change their password.
A password change requires the current password and a new password. After a successful change, all sessions belonging to that user should be invalidated.
This means the user may need to sign in again on other devices.
Use a unique password that is not shared with other services. Since HomeLite does not implement two-factor authentication, password quality and protection of the server are especially important.
Keyboard Shortcuts
Keyboard shortcuts are not part of the documented feature set. Do not rely on shortcuts unless they are implemented and documented in the current release.
A future UI revision may introduce shortcuts for common actions, such as focusing search or opening the upload picker. Such shortcuts should be documented only after implementation and testing.
API Reference
HomeLite exposes a JSON HTTP API alongside its browser interface.
The API is intended for use with the same server-side authentication and authorization rules as the dashboard.
General conventions
- JSON request bodies should use Content-Type: application/json.
- Upload requests use multipart/form-data.
- Successful responses use JSON unless an endpoint streams file content.
- Authenticated requests use the session cookie issued by login.
- Mutating authenticated requests must include X-CSRF-Token.
- API errors use a JSON object with error and message fields.
- File IDs and parent IDs are logical database identifiers, not filesystem paths.
The endpoint tables describe the intended API surface. Verify exact response fields and identifier types against the implementation before using the API as a stable third-party integration contract.
Public Endpoints
These endpoints do not require an authenticated session.
Method	Path	Request	Success response
GET	/api/status	None	Server name, setup state, counts, size, uptime, LAN IP, port, bytes served, maximum upload size.
POST	/api/setup	{"username":"admin","password":"..."}	HTTP 201, {"ok":true}
POST	/api/login	{"username":"admin","password":"..."}	{"ok":true,"csrf":"..."}
POST	/api/logout	Session cookie and CSRF token when required by the implementation	{"ok":true}
GET /api/status
Returns operational information intended for the first-run page and status display.
The response includes:
- Server display name.
- Whether initial setup is required.
- File and user counts.
- Total stored size.
- Server uptime.
- Detected LAN IP.
- Listening port.
- Bytes served.
- Maximum upload size.
Treat operational status as public information. Do not extend this endpoint to expose usernames, password hashes, session tokens, or private file listings.
POST /api/setup
Request:
{
  "username": "admin",
  "password": "replace-with-a-strong-password"
}


Success:
{
  "ok": true
}


Expected successful status: 201 Created.
This endpoint must reject setup attempts after an administrator has already been created.
POST /api/login
Request:
{
  "username": "admin",
  "password": "replace-with-your-password"
}


Success:
{
  "ok": true,
  "csrf": "session-csrf-token"
}


The server also issues a session cookie. Store and reuse the cookie according to the HTTP client's cookie handling rules.
Login throttling is applied per username and IP address. Excessive attempts should return HTTP 429.
POST /api/logout
Logs out the current session.
The server should invalidate the session and clear the browser cookie. Include the CSRF token if required by the application's mutating-request policy.
Authentication and Profile
All endpoints in this group require an authenticated session.
Method	Path	Request	Success response
GET	/api/me	None	{"user":{...},"csrf":"..."}
POST	/api/me/password	{"old_password":"...","new_password":"..."}	{"ok":true}
GET /api/me
Returns the current user's profile and CSRF token.
Example response shape:
{
  "user": {
    "id": 1,
    "username": "admin",
    "role": "admin"
  },
  "csrf": "session-csrf-token"
}


The fields shown in the example illustrate the response shape; consult the actual implementation for the complete profile fields.
POST /api/me/password
Headers:
Content-Type: application/json
X-CSRF-Token: session-csrf-token


Request:
{
  "old_password": "current-password",
  "new_password": "new-strong-password"
}


Success:
{
  "ok": true
}


The server must verify the current password, hash the new password using the configured Argon2id parameters, and invalidate the user's sessions.
File Operations
These endpoints require an authenticated session. Mutating endpoints require a valid CSRF token.
Method	Path	Request	Success response
GET	/api/files	Query parameters: parent_id, q, sort, order	{"files":[...],"user":{...}}
POST	/api/files/mkdir	{"name":"Documents","parent_id":null}	{"ok":true,"id":"..."}
POST	/api/files/upload	Multipart fields: parent_id, file	{"ok":true,"id":"...","name":"...","size":123,"sha256":"..."}
POST	/api/files/:id/rename	{"name":"new-name.txt"}	{"ok":true}
POST	/api/files/:id/move	{"parent_id":"..."}	{"ok":true}
DELETE	/api/files/:id	None	{"ok":true}
GET	/api/files/:id/download	None	Streamed file response
GET /api/files
Query parameters:
Parameter	Purpose
parent_id	Logical parent folder ID. An omitted or empty value represents the root according to the implementation's query convention.
q	Filename search query.
sort	Sort field: name, size, or upload date.
order	Sort direction: ascending or descending.
Example:
GET /api/files?parent_id=&q=report&sort=name&order=asc


The server must return only files and folders belonging to the authenticated user. Search and sorting must not bypass ownership checks.
POST /api/files/mkdir
Request:
{
  "name": "Documents",
  "parent_id": null
}


Creates a folder under the requested parent. The server validates the name and confirms that the parent belongs to the authenticated user.
POST /api/files/upload
The request uses multipart/form-data.
Required fields:
Field	Type	Description
parent_id	Logical folder ID or supported root representation	Destination folder.
file	File part	Contents to upload.
Success:
{
  "ok": true,
  "id": "file-id",
  "name": "report.pdf",
  "size": 12345,
  "sha256": "hexadecimal-sha256-digest"
}


The server streams the file to temporary storage, computes SHA-256 while receiving it, and commits the file metadata and storage object according to the implementation's transaction strategy.
Upload limits include the per-file limit, per-user quota, and maximum four simultaneous uploads.
POST /api/files/:id/rename
Request:
{
  "name": "renamed-report.pdf"
}


The ID in the URL identifies a logical file or folder record. The server validates ownership and the new name.
POST /api/files/:id/move
Request:
{
  "parent_id": "destination-folder-id"
}


Moves the item to a destination folder belonging to the same user. The implementation must prevent invalid moves, including moves that would create a folder cycle.
DELETE /api/files/:id
Deletes the identified file or folder after checking ownership.
Folder deletion is recursive. The implementation must handle filesystem errors and metadata consistency carefully.
GET /api/files/:id/download
Returns the file content as a stream rather than a JSON object.
The response should include a suitable Content-Type inferred from the filename extension and a Content-Disposition header preserving the original filename safely.
The client must not supply a physical storage path or determine the path used by the server.
Administration API
These endpoints require an authenticated administrator session. Mutating requests require X-CSRF-Token.
Method	Path	Request	Success response
GET	/api/admin/users	None	{"users":[...]}
POST	/api/admin/users	{"username":"...","password":"...","role":"user","quota_bytes":53687091200}	{"ok":true}
PATCH	/api/admin/users/:id	Partial user fields	Implementation-defined success response
DELETE	/api/admin/users/:id	None	{"ok":true}
GET	/api/admin/settings	None	{"key":"value"}
PUT	/api/admin/settings	Partial settings object	{"ok":true,"restart_required_for":[]}
GET /api/admin/users
Returns the users available for administration.
User listings must not include password hashes, active session tokens, CSRF tokens, or other authentication secrets.
POST /api/admin/users
Request:
{
  "username": "alice",
  "password": "a-strong-unique-password",
  "role": "user",
  "quota_bytes": 53687091200
}


Creates a user account with the supplied role and quota.
Validate usernames, passwords, role values, and quota values on the server. The client must not be allowed to grant administrator privileges unless the authenticated caller is authorized to do so.
PATCH /api/admin/users/:id
Supported partial fields:
{
  "disabled": false,
  "quota_bytes": 53687091200,
  "role": "user",
  "new_password": "replacement-password"
}


Only supplied fields should be modified. A request may contain one or several supported fields.
The server must validate every field and prevent unauthorized role changes. Password resets should invalidate the affected user's sessions.
DELETE /api/admin/users/:id
Deletes the selected user account according to the implementation's account-deletion policy.
Do not assume that deleting an account securely erases every copy of its files from disk or from external backups.
GET /api/admin/settings
Returns configurable application settings.
PUT /api/admin/settings
Example request:
{
  "server_display_name": "Home Archive",
  "session_hours": 72,
  "theme": "dark"
}


Example response:
{
  "ok": true,
  "restart_required_for": []
}


The restart_required_for array identifies settings that need a server restart. Port changes are expected to require a restart.
Errors
API errors use a consistent JSON structure:
{
  "error": "invalid_request",
  "message": "The supplied request is invalid."
}


The error value is a machine-readable code. The message value is intended for humans.
HTTP status	Meaning	Typical situation
400	Bad Request	Invalid JSON, malformed input, or invalid parameter.
401	Unauthorized	Missing or invalid session.
403	Forbidden	Insufficient permissions or invalid CSRF token.
404	Not Found	File, folder, or resource does not exist or is not accessible.
409	Conflict	Duplicate username, conflicting operation, or invalid state transition.
413	Content Too Large	Upload exceeds the per-upload limit.
429	Too Many Requests	Login throttling or another implemented request limit.
500	Internal Server Error	Unexpected server-side failure.
507	Insufficient Storage	User quota or available storage prevents the upload.
Exact status codes may depend on the implementation and the operation. Clients should handle errors using the HTTP status and JSON response rather than assuming every failure has the same code.
Accessing From Other Devices
HomeLite can serve other devices on the same local network. The server computer and client device must be able to communicate over the network, and the configured port must be reachable.
1. Find the server's LAN address
Start HomeLite and check the terminal banner for the displayed LAN URL.
A URL might look like:
http://192.168.1.25:8080


This is an example address, not a guaranteed address for your network.
If no LAN address is displayed, inspect the network settings on the host computer.
Windows:
ipconfig


Linux:
ip addr


macOS:
ifconfig


Look for the IPv4 address of the interface connected to the same network as your phone or computer. Avoid using loopback addresses such as 127.0.0.1 from another device.
2. Verify the listening interface
HomeLite must listen on a network-accessible interface for LAN clients to connect.
If it listens only on 127.0.0.1, other devices cannot connect directly. Check the HTTP server's bind address in the implementation.
The advertised LAN URL is useful only if the server is actually listening on an appropriate interface.
3. Open the URL on another device
Connect the phone, tablet, or second computer to the same Wi-Fi or LAN.
Open the server URL in a browser:
http://192.168.1.25:8080


Replace the example IP with the actual address shown by HomeLite or the host's network configuration.
Sign in with an account created by the administrator.
4. Configure Windows Firewall
If Windows Firewall blocks inbound connections, create a rule for the selected TCP port.
Open PowerShell as Administrator:
New-NetFirewallRule `
  -DisplayName "HomeLite LAN HTTP" `
  -Direction Inbound `
  -Protocol TCP `
  -LocalPort 8080 `
  -Action Allow `
  -Profile Private


This rule permits inbound TCP traffic on port 8080 for the Private network profile.
Do not enable this rule for every network profile without considering the security implications. A Private profile should only be used for networks you trust.
To remove the rule:
Remove-NetFirewallRule -DisplayName "HomeLite LAN HTTP"


If you change HomeLite's listening port, update the firewall rule accordingly.
5. Test connectivity
From a Windows client, try opening the URL in a browser. You can also test the TCP port from PowerShell:
Test-NetConnection 192.168.1.25 -Port 8080


Replace the example address with the server's actual IP.
A successful TCP connection establishes that the port is reachable; it does not prove that login, file access, or every API endpoint works correctly.
Troubleshooting checklist
- [ ] HomeLite is running.
- [ ] The browser URL uses the correct server IP.
- [ ] The port in the URL matches the configured port.
- [ ] The server listens on a LAN-accessible interface.
- [ ] The client and server can communicate on the LAN.
- [ ] Windows Firewall or another host firewall permits the port.
- [ ] The router does not enable client isolation between Wi-Fi devices.
- [ ] The network does not isolate wireless clients from wired clients.
- [ ] The server's IP address has not changed.
- [ ] No other process is occupying the selected port.
Remote Access
HomeLite is designed primarily for use on a trusted local network. Remote access introduces additional network and security requirements.
Recommended: VPN
A VPN allows an authorized device to reach the home network or a designated server without exposing HomeLite's HTTP port directly to the public internet.
A VPN-based deployment should:
1. Restrict VPN access to authorized devices and users.
2. Keep HomeLite's listening port inaccessible from untrusted networks.
3. Use strong authentication for both the VPN and HomeLite.
4. Keep the server and VPN software updated.
5. Verify that the VPN's routing and firewall rules prevent unintended access to other local services.
A VPN does not encrypt files at rest and does not replace HomeLite's authentication and authorization controls.
Alternative: Cloudflare Tunnel
A Cloudflare Tunnel can publish a local web service through an outbound tunnel rather than requiring a direct inbound port-forwarding rule.
However, publishing HomeLite through a tunnel changes the threat model. The service becomes reachable through a public hostname, and configuration errors can expose login pages, files, or administrative endpoints.
Before using a tunnel:
- Configure HTTPS at the public endpoint.
- Understand where TLS terminates and whether traffic from the tunnel connector to HomeLite is still HTTP.
- Apply access controls appropriate to the deployment.
- Test access to all routes, including the setup page and administrator API.
- Avoid exposing the service until initial setup is complete.
- Review the tunnel provider's privacy, access, and logging policies.
- Keep HomeLite and its host updated.
- Ensure that trusted proxy headers cannot be spoofed by untrusted clients.
A tunnel does not automatically make the application secure. In particular, cookie handling that trusts X-Forwarded-Proto: https must only rely on that header when requests can come through a trusted proxy path. If arbitrary clients can reach the backend directly and supply the header, cookie security assumptions can fail.
Do not port-forward plain HTTP
Security warning: Do not forward HomeLite's HTTP port directly to the public internet. Credentials, session cookies, and file contents can travel in plaintext over HTTP. A public deployment requires a correctly configured HTTPS reverse proxy or a VPN-based access design, along with appropriate access controls.

HTTPS protects data in transit but does not protect files stored unencrypted on the server. See Security Model.
Backup and Restore
A complete backup must preserve both the database and the stored file contents.
Do not copy only homelite.db while HomeLite is actively writing to it. SQLite WAL mode can leave committed changes in the WAL file until checkpointing. Use SQLite's backup mechanism for the database, and coordinate the filesystem copy with the server.
Recommended backup procedure
For the simplest consistent backup:
1. Stop HomeLite gracefully.
2. Wait for the process to exit.
3. Back up the complete files/ directory.
4. Back up homelite.db.
5. Preserve the backup in a separate location or on separate storage.
6. Restart HomeLite if required.
A graceful shutdown allows the application to perform its WAL checkpoint. The database backup should still be validated before relying on it.
For a live database backup, use SQLite's online backup mechanism, such as the SQLite CLI .backup command, and coordinate the file copy so that the database snapshot and filesystem contents represent a consistent point in time.
Back up the database with SQLite
If the SQLite command-line tool is available, run the backup command while the server is running:
sqlite3 data/homelite.db ".backup 'homelite-backup.db'"


This creates a database backup using SQLite's backup interface rather than copying the live database file directly.
The command assumes that the sqlite3 executable is installed. It is a maintenance tool, not a HomeLite runtime dependency.
On Windows, run the equivalent command from Command Prompt if the SQLite CLI is installed and available in PATH:
sqlite3 data\homelite.db ".backup 'homelite-backup.db'"


The destination should be outside the active database directory or otherwise managed carefully to avoid overwriting the live database.
Back up the filesystem
After taking a consistent database snapshot, copy the corresponding stored files.
Linux or macOS:
cp -a data/files ./homelite-files-backup


Windows PowerShell:
Copy-Item -Recurse -Force .\data\files .\homelite-files-backup


For an application that is actively accepting uploads, copying the files while uploads or deletes are in progress can create an inconsistent backup. For the most predictable result, stop HomeLite before copying the filesystem or use a maintenance procedure that prevents changes during the backup.
Restore
1. Stop HomeLite.
2. Preserve the current data directory in case rollback is necessary.
3. Restore the backed-up homelite.db.
4. Restore the matching files/ directory.
5. Confirm that the restored paths and permissions are correct.
6. Set HOMELITE_DATA_DIR if restoring to a different location.
7. Start HomeLite.
8. Sign in and verify several users, folders, and files.
9. Download test files and compare their contents with known originals.
10. Check the database integrity.
Do not combine a database backup from one point in time with a filesystem backup from an unrelated point in time. Files uploaded or deleted between those snapshots may be missing or orphaned.
Verify database integrity
Use SQLite's integrity check on a stopped or safely backed-up database:
sqlite3 homelite-backup.db "PRAGMA integrity_check;"


A healthy database should return:
ok


The check verifies database structural integrity. It does not prove that every referenced file exists, that every stored file matches its recorded SHA-256 checksum, or that the backup can be restored successfully.
Test recovery periodically
A backup is only useful if it can be restored.
Periodically restore a copy to a separate directory, start HomeLite against it in an isolated environment, and verify that:
- Accounts and settings are present.
- File listings match the expected state.
- Downloaded files have the expected contents.
- User isolation remains intact.
- The application can accept new uploads.
Keep at least one backup separate from the host running HomeLite. A backup stored only on the same physical disk does not protect against disk failure.
Security Model
HomeLite includes application-level controls for authentication, authorization, storage isolation, and request validation. Those controls reduce risk but do not make the host, network, or stored files immune to compromise.
Implemented security controls
Control	Intended behavior
Password hashing	Argon2id with 64 MiB memory, time cost 2, parallelism 4, and 32-byte output.
Server-side sessions	Session records are stored in SQLite with a configurable lifetime.
Session fixation prevention	A new session token is issued at login.
CSRF protection	Mutating requests require a session-specific token in X-CSRF-Token, checked in constant time.
Cookie protection	HttpOnly, SameSite=Lax, and conditional Secure attributes.
Login throttling	Eight attempts per 15 minutes per username and IP, using in-memory counters.
Ownership checks	Protected file operations verify that the resource belongs to the authenticated user.
Role checks	Administrative endpoints require administrator authorization.
Path traversal prevention	Clients supply logical IDs rather than physical filesystem paths.
Filename validation	Reject path separators, null bytes, and prohibited .. sequences.
MIME type handling	Infer content types from extensions rather than trusting a client-supplied Content-Type.
SQL injection prevention	Parameterized SQL queries.
Upload integrity	SHA-256 checksum calculation during streaming upload.
HTTP response headers	X-Content-Type-Options: nosniff, X-Frame-Options: DENY, and Referrer-Policy: same-origin.
Storage quotas	Per-user quotas enforced during upload.
Resource bounds	Maximum four simultaneous uploads.
Database configuration	SQLite WAL mode, foreign keys, and a 10-second busy timeout.
These descriptions specify the intended controls. Review the actual source code and test the relevant paths before treating them as verified security guarantees for a particular release.
Honest limitations
No end-to-end encryption
HomeLite does not implement end-to-end encryption. Stored files are plaintext on the host filesystem.
Anyone who obtains sufficient access to the host or its storage may be able to read the files independently of HomeLite's login system.
Recommendation: Enable full-disk encryption on the host using BitLocker, FileVault, or LUKS, as appropriate for the operating system. Use encrypted backups as well.
Full-disk encryption protects against some forms of physical storage theft but does not prevent a compromised, unlocked host from accessing the data.
No resumable uploads
Uploads are not resumable. If an upload is interrupted, it must start again.
Large uploads are particularly vulnerable to network interruptions, browser shutdown, device sleep, and server restarts.
No two-factor authentication
HomeLite uses password-based authentication without built-in TOTP or another second factor.
Use strong, unique passwords, restrict network access, and protect the host.
No antivirus or content scanning
HomeLite does not scan uploads for malware. A stored file is not guaranteed to be safe merely because the upload succeeded.
Do not open untrusted downloads on sensitive systems without appropriate scanning and precautions.
Login throttling is in memory
Login throttling is maintained in process memory. Restarting HomeLite clears the counters.
The counters are not shared between multiple HomeLite processes. They are not a substitute for network-level protection or a persistent abuse-prevention system.
HTTP is not encrypted
The default HTTP server does not encrypt credentials, session cookies, or file contents in transit.
Use HomeLite over a trusted LAN or behind a properly configured HTTPS reverse proxy or VPN.
Startup sound may be blocked
Browsers commonly prevent audio autoplay until the user interacts with the page. The embedded /startup.mp3 sound may not play on the first visit.
This is expected browser behavior, not necessarily a server error.
Deployment recommendations
- Use a supported and updated operating system.
- Keep HomeLite updated to a reviewed release.
- Use full-disk encryption.
- Restrict access to the data directory.
- Run HomeLite under a dedicated, unprivileged operating-system account where practical.
- Keep the database and file directory writable only by the account that needs them.
- Use a trusted LAN, VPN, or correctly configured HTTPS reverse proxy.
- Do not expose the default HTTP port directly to the public internet.
- Complete first-run setup before enabling remote access.
- Use strong, unique account passwords.
- Create separate accounts for separate users.
- Apply sensible quotas to prevent a single user from consuming all available storage.
- Back up both SQLite metadata and stored files.
- Test restore procedures.
- Monitor disk capacity.
- Avoid running multiple instances against the same data directory.
- Review proxy-header trust and cookie settings before remote deployment.
Performance and Resource Limits
HomeLite is designed around streamed file I/O and a lightweight embedded frontend. Actual performance depends on the hardware, storage device, network, Go build, SQLite workload, and size and number of files.
Memory use
HomeLite does not intentionally buffer entire files in memory during uploads or downloads.
However, it would be misleading to promise a fixed idle memory footprint without measurements from a particular build and workload.
Memory consumption can vary with:
- Number of concurrent connections.
- Number of simultaneous uploads.
- Multipart parsing and buffer sizes.
- SQLite page cache and connection pool configuration.
- Number of file metadata records.
- Response sizes.
- Operating-system filesystem caching.
- Embedded assets and runtime allocations.
Measure the actual executable on the intended host rather than relying on an assumed idle-memory figure.
Streaming behavior
Uploads stream to temporary disk storage while SHA-256 is calculated. Downloads stream from a file handle using io.Copy.
This design avoids reading an entire large file into a single application buffer. It does not eliminate disk I/O, network buffering, filesystem cache usage, or the memory required by active requests.
Interrupted uploads are not resumable. Temporary files left by interrupted operations are intended to be cleaned up at startup.
Upload concurrency
The maximum number of simultaneous uploads is four.
Additional requests may wait for an upload slot or be handled according to the implementation's semaphore behavior.
A bounded upload count helps limit concurrent resource consumption but does not provide a complete request-rate or connection limit.
Per-upload and per-user limits
Limit	Default
Maximum individual upload	5 GiB
Default per-user quota	50 GiB
Simultaneous uploads	4
SQLite busy timeout	10 seconds
Default session lifetime	72 hours
The host's free disk space can be lower than the combined quotas. Quotas are logical account limits, not disk reservations.
SQLite and connection pooling
HomeLite uses SQLite in WAL mode with foreign keys enabled and a 10-second busy timeout.
The busy timeout allows SQLite to wait for a lock for a limited period. It does not guarantee that every transaction will succeed under contention.
The exact SQLite connection-pool size is an implementation detail that should be confirmed in the database initialization code. Do not assume a particular pool size based solely on the use of SQLite or Go's database/sql.
If you change pool settings, test:
- Concurrent reads and writes.
- Upload completion and metadata commits.
- User creation and password changes.
- Folder moves and recursive deletes.
- Graceful shutdown and WAL checkpointing.
- Behavior under a sustained upload workload.
Practical tuning
For a small home deployment:
1. Use an SSD when available.
2. Keep the data directory on a reliable disk.
3. Avoid unnecessary concurrent HomeLite instances.
4. Ensure adequate free disk space.
5. Use a stable wired network for large uploads when practical.
6. Set quotas according to available storage.
7. Back up the database and files regularly.
8. Measure performance before changing SQLite or HTTP settings.
Do not raise concurrency limits without considering disk throughput, network bandwidth, database contention, and the number of users.
Project Structure
The following is a reference layout for the intended implementation. Match it to the actual repository before publishing the README; Go package and file names depend on how the source is organized.
HomeLite/
├── go.mod
├── go.sum
├── main.go
├── database.go
├── schema.go
├── auth.go
├── sessions.go
├── csrf.go
├── users.go
├── files.go
├── storage.go
├── settings.go
├── middleware.go
├── handlers.go
├── embedded.go
├── index.html
├── logo.ico
├── startup.mp3
├── LICENSE
├── README.md
├── docs/
│   └── screenshots/
│       ├── dashboard.png
│       ├── dashboard-dark.png
│       ├── mobile.png
│       ├── upload-progress.png
│       ├── admin-users.png
│       └── settings.png
└── data/
    ├── homelite.db
    ├── files/
    └── temp/


File or directory	Intended responsibility
go.mod	Module name and dependency declarations.
go.sum	Checksums for Go module dependencies.
main.go	Program entry point, configuration loading, startup, and shutdown.
database.go	SQLite connection setup and database helpers.
schema.go	Database schema creation and migrations.
auth.go	Password hashing and credential verification.
sessions.go	Server-side session creation, validation, expiration, and invalidation.
csrf.go	CSRF token creation and validation.
users.go	User records, roles, quotas, and administrative operations.
files.go	File metadata and logical file and folder operations.
storage.go	Filesystem operations, streaming, and temporary uploads.
settings.go	Reading, validating, and persisting settings.
middleware.go	Authentication, authorization, and shared HTTP middleware.
handlers.go	HTTP API endpoint handlers.
embedded.go	Embedded resource declarations and static asset serving.
index.html	Single-file HTML, CSS, and vanilla JavaScript dashboard.
logo.ico	Embedded favicon.
startup.mp3	Embedded startup chime.
LICENSE	MIT license text.
docs/screenshots/	Actual project screenshots.
data/	Runtime database and managed file storage.
The final source tree may combine these responsibilities into fewer files. This list describes logical components, not a requirement to create one file per component.
Embedded assets
The frontend and static assets are compiled into the executable through Go's //go:embed directive.
A typical declaration might resemble:
package main

import "embed"

//go:embed index.html logo.ico startup.mp3
var embeddedAssets embed.FS


This is an illustrative example. The actual embed declaration must match the project's package structure and asset paths.
If an asset is not present at the declared path, the build fails. Once an executable has been built, changing an external copy of the asset does not update the embedded resource.
Troubleshooting
Error or symptom	Likely cause	Fix
bind: address already in use or Only one usage of each socket address...	Another process is using the configured port.	Change the port in settings and restart HomeLite, or stop the conflicting process.
Dashboard works on the server but not from a phone	Firewall blocks inbound traffic, server binds only to loopback, or the network isolates clients.	Verify the bind address, LAN IP, port, firewall rule, and router client-isolation settings.
permission denied while opening the data directory	The current account cannot read or write the configured directory.	Correct directory ownership and permissions, then restart HomeLite.
pattern ...: no matching files found during build	A //go:embed pattern references a missing file or directory.	Confirm that every embedded asset exists at the path specified by the directive.
database is locked	Concurrent transactions, another HomeLite process, or a long-running transaction is holding a SQLite lock.	Ensure only the intended instance uses the data directory, investigate long transactions, and review connection-pool settings.
Upload stalls or fails near completion	Network interruption, disk space exhaustion, quota exhaustion, server restart, or request timeout.	Check server logs/output, disk space, user quota, and network stability. Retry the upload; interrupted uploads are not resumable.
Favicon does not update	Browser cache or an older executable still contains the previous embedded icon.	Rebuild the executable with the new asset, restart it, and refresh or clear the browser cache.
Startup sound does not play	Browser autoplay policy blocks audio before user interaction.	Interact with the page and retry. This behavior is expected on browsers that restrict autoplay.
Login returns 401	Invalid credentials, missing session, expired session, or invalidated session.	Sign in again and verify the username and password.
Mutating request returns 403	Missing, expired, or invalid CSRF token; insufficient privileges.	Obtain the current token from the authenticated session and send it in X-CSRF-Token.
Login returns 429	The username/IP combination exceeded the in-memory login attempt threshold.	Stop retrying and allow the throttling window to pass. Restarting clears the counters but should not be used as a security workaround.
Upload returns 413	The file exceeds the configured maximum upload size.	Choose a smaller file or increase max_upload_bytes if the host can safely support it.
Upload returns 507	The user quota is exhausted or the application cannot accept the upload due to insufficient storage.	Check the user's quota and the host's available disk space.
A file or folder returns 404	Incorrect ID, deleted resource, or resource not accessible to the current user.	Refresh the listing and verify that the correct account is signed in.
A setting does not take effect immediately	The setting requires a restart or the UI has not refreshed its state.	Check restart_required_for, restart when needed, and reload the dashboard.
Files appear missing after moving the data directory	HomeLite started with an empty or incomplete data directory.	Stop the server and restore the matching database and files/ directory to the configured location.
SQLite reports malformed or corrupt data	Incomplete manual copy, storage failure, or database corruption.	Stop HomeLite, preserve the affected files, restore a known-good backup, and run PRAGMA integrity_check.
Windows terminal animation shows incorrect colors	Windows console ANSI support is unavailable or not enabled.	Use Windows 10 or later with a supported console. Run with --no-anim if necessary.
HomeLite stops when the terminal closes	The process is attached to the terminal session.	Use an appropriate service manager or supervised process configuration for the operating system.
LAN URL changes after reboot	The router assigned a different DHCP address.	Reserve a DHCP address for the host or update the client URL.
Build error: missing embedded assets
Go requires files matched by //go:embed patterns to exist at compile time.
Check the repository before rebuilding:
ls


On Windows:
dir


Confirm that index.html, logo.ico, and startup.mp3 exist wherever the source code expects them.
Do not remove required embedded assets simply to make the build pass. If an asset is intentionally optional, update the embed declaration and serving logic together.
Database lock errors
SQLite allows multiple readers and coordinates writes. WAL mode improves reader/writer concurrency but does not eliminate write contention.
Check that:
- Only one intended HomeLite instance uses the directory.
- Transactions are committed or rolled back promptly.
- Database rows and filesystem changes are not left in inconsistent states.
- The process has sufficient filesystem permissions.
- The disk is healthy and not reporting I/O errors.
Do not delete homelite.db-wal or homelite.db-shm manually while HomeLite or another SQLite connection is using the database.
Upload failures
For a failed upload, check these conditions in order:
1. The client remains connected.
2. The server is still running.
3. The file is below the per-upload limit.
4. The destination folder belongs to the current user.
5. The user's quota has sufficient remaining capacity.
6. The data directory has enough free space.
7. The server can create and write temporary files.
8. No conflicting process is using the same data directory.
If an upload was interrupted, retry it from the beginning.
FAQ
Question	Answer
Can I access HomeLite over the internet?	Yes, through a carefully configured VPN or HTTPS reverse proxy. HomeLite is designed primarily for LAN use. Do not port-forward its plain HTTP port directly to the public internet.
Are my files encrypted?	Files are plaintext on the host filesystem. Use BitLocker, FileVault, or LUKS for full-disk encryption, and use encrypted backups when appropriate.
Can I run HomeLite on a Raspberry Pi?	Go supports Raspberry Pi-compatible Linux architectures. Build for the correct OS and architecture, then test the release on the device. Storage performance and available RAM depend on the model and workload.
Can I move the data directory?	Yes. Stop HomeLite, migrate the database and matching files/ directory, set HOMELITE_DATA_DIR, and verify the restored data before removing the original copy.
Can I have multiple administrator users?	The user-management API supports role assignment, so multiple administrators are possible if the implementation permits creating and assigning more than one administrator account. Verify the role-validation rules in the release you use.
What happens if I press Ctrl-C during an upload?	HomeLite attempts graceful shutdown on supported termination signals. An active upload may fail or be interrupted; uploads are not resumable. Temporary files are intended to be cleaned up at startup.
Does HomeLite work on Windows 10?	The intended Windows executable is homelite.exe. Windows 10 or later is recommended for ANSI terminal colors. Verify the downloaded release's architecture and requirements.
Does HomeLite need Docker?	No. Docker is not required at runtime or for a normal Go build.
Does HomeLite need Node.js or Python?	No. The dashboard uses HTML, CSS, and vanilla JavaScript. The server is written in Go.
Where are passwords stored?	Passwords should be stored as Argon2id hashes in SQLite, not as plaintext. The intended parameters are 64 MiB memory, time cost 2, parallelism 4, and 32-byte output.
How do I reset a forgotten administrator password?	If another administrator can access the panel, they can reset it. Otherwise, use a documented recovery procedure for the exact release. Do not delete database records or edit hashes blindly. If no recovery utility exists, stop the service, preserve a backup, and seek a source-reviewed recovery method.
Can I change the port?	Yes. Change the port setting through administration and restart HomeLite. Update firewall rules and client URLs if needed.
Is there a mobile app?	The documented interface is a responsive web dashboard. A separate native mobile application is not part of the specified feature set.
What happens if my laptop sleeps?	The server becomes unavailable while the machine sleeps or loses network connectivity. Clients cannot reliably upload or download files until it resumes.
How do I update to a new version?	Back up the database and files, stop HomeLite, replace the executable with a trusted release, and restart it. Schema migrations are intended to run automatically. Verify the result and keep the previous executable and backup until the update is confirmed.
Can multiple people use HomeLite at the same time?	Yes, through separate user accounts. File access must remain isolated by user ownership. Actual performance depends on the hardware and workload.
Can I upload a file larger than 5 GiB?	Not with the default per-upload limit. An administrator can change max_upload_bytes, but the filesystem, available disk space, user quota, and implementation limits still apply.
Does searching inspect file contents?	No. The documented search feature filters filenames across folders.
Can I run two HomeLite instances against the same database?	Do not do this unless multi-instance operation is explicitly supported and tested. SQLite coordination alone does not make every application-level filesystem operation safe across instances.
Does deleting a file securely erase it?	No secure-erasure guarantee is documented. Deletion is not equivalent to securely overwriting storage or removing copies from backups.
Does HomeLite send telemetry?	The project is designed without telemetry or a mandatory third-party storage service. Review the actual source and dependencies to verify this claim for a specific build.
License
HomeLite is licensed under the MIT License.
See the LICENSE file for the complete license text and applicable terms.
