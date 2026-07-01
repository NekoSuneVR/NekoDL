# NekoDL — Roadmap / TODO

This is the working plan for building NekoDL: an aria2-inspired download manager with native yt-dlp, VPN/proxy-shielded torrenting, BoothDownloader and Plex-ripping engines, *arr/Ombi/Tdarr media-stack integration, and a Tunarr-style live-TV module, all behind a Web GUI and Docker.

Legend: `[ ]` not started · `[~]` in progress · `[x]` done

---

## Phase 0 — Planning & Decisions

- [x] Write initial README.md, TODO.md, CHANGELOG.md
- [x] Decide final project license → **MIT**. Permissive, compatible with BoothDownloader (Apache-2.0, used only as a wrapped/invoked dependency, not vendored source) and yt-dlp (Unlicense).
- [x] Decide core language/runtime → **Go**, confirmed.
- [x] Decide BitTorrent library → **`anacrolix/torrent`**, confirmed as the plan; final validation of custom-dialer proxy binding happens when Phase 3 starts and a Go toolchain is available to build against it.
- [x] Decide how BoothDownloader is invoked → **shell out to its released CLI binary** for v1 (simplest, reuses its working auth flow); native reimplementation stays a possible later optimization to drop the runtime dependency.
- [x] Decide config format → **JSON** for v1 (`nekodl.json`), parsed with the Go standard library only, no third-party dependency required to get the core running before a build pipeline exists. YAML remains an open revisit once tooling is in place.
- [x] Decide RPC approach → NekoDL's own REST/WebSocket API is primary (Phase 1); the aria2-compatible JSON-RPC shim (needed for Radarr/Sonarr/Lidarr) is an additional compatibility layer built in Phase 12, not the core protocol.
- [x] Pick a repo layout → **monorepo**: root docs + `core/` (Go backend) + `web/` (React dashboard, added when Phase 7 starts) + `docker/` (added when Phase 8 starts).

## Phase 1 — Core Engine Skeleton

- [x] Scaffold core service — minimal Go module (`core/`) with a JSON config loader, stdlib `net/http` server, `/health` endpoint, and graceful shutdown (`core/cmd/nekodl`, `core/internal/config`, `core/internal/api`). **Verified**: Go 1.26 toolchain installed, `go build ./...` and `go vet ./...` both pass clean.
- [x] Define the internal `Task` interface (`core/internal/task/task.go`) — settled on `Pause/Resume/Cancel/Remove/Progress/Status`; dropped `Add` from the interface itself since adding is a scheduler operation that constructs a task, not a method on an existing one.
- [x] Implement task queue + scheduler (`core/internal/scheduler`) — global `MaxConcurrentDownloads` limit, priority ordering among pending tasks (higher priority + older first). Per-task `MaxBandwidthBps` is accepted and stored but **not enforced yet** — actual rate limiting is an engine-level concern once a real engine (Phase 2+) exists to throttle.
- [x] Implement persistent task/session storage (`core/internal/scheduler/store.go`) — JSON snapshot of task records (`tasks.json` in `data_dir`) written on every mutation. Scoped honestly: this persists *metadata* (id/engine/status/progress) for history; it can't reconstruct a live, resumable task on restart since no real engine exists yet to reattach to. Real resume-after-restart is an engine-level concern for later phases.
- [x] Define REST API contract (`core/internal/api`) — `GET/DELETE /api/v1/tasks[/{id}]`, `POST /api/v1/tasks/{id}/{pause,resume,cancel}`, using Go 1.22's stdlib `net/http.ServeMux` method+wildcard routing (no router dependency needed).
- [x] Live progress events — real WebSocket at `GET /api/v1/events`, hand-rolled against RFC 6455 using only the Go standard library (`core/internal/api/websocket.go`: SHA-1/base64 handshake via `net/http.Hijacker`, manual frame read/write, ping/pong handling). **Verified live**, not just compiled: ran the server and connected a real, independent WebSocket client (via the Monitor tool's `ws` source) to `ws://localhost:6900/api/v1/events` — the handshake completed and the client correctly decoded 5 pushed JSON text frames before a clean timeout. That confirms the handshake hash and frame length/masking encoding are actually RFC-compliant, not just self-consistent with this codebase's own (potentially symmetrically-wrong) reader.
- [x] Basic auth / API token support — `Authorization: Bearer <token>` checked with constant-time comparison (`crypto/subtle`) in `core/internal/api/auth.go`; `/health` stays unauthenticated for external monitors. Empty `api_token` in config disables auth (logged as a warning on startup).
- [x] Unit tests for scheduler and task lifecycle (`core/internal/scheduler/scheduler_test.go`) — covers enqueue/ordering, concurrency-limit enforcement, priority ordering, not-found errors, remove, and store save/load round-trip, using a test-only fake `task.Task`. **Verified**: `go test ./...` — all 7 tests pass.

## Phase 2 — HTTP/FTP Engine

- [x] Segmented multi-connection HTTP/HTTPS downloading with resume (`core/internal/httpengine`) — probes the server for size + Range support, splits into up to `MaxConnections` segments (min 1 MiB each), downloads them concurrently, and persists per-segment resume offsets to a `<dest>.nekodl-progress.json` sidecar file after every chunk. Falls back to a single unranged connection (no mid-file resume possible) when the server doesn't support `Range`.
- [x] Checksum verification (MD5/SHA-1/SHA-256) — `core/internal/httpengine/checksum.go`, runs after all segments complete; a mismatch fails the task with a clear error instead of silently keeping a corrupt file.
- [x] Retry/backoff policy for flaky connections — exponential-ish backoff (`(attempt+1)*500ms`), applied to **both** the initial probe and each segment fetch. (The first version only retried segments, not the probe — caught by `TestRetrySucceedsAfterTransientFailure` failing, see Notes.)
- [x] Mirror/fallback URL support — a task takes an ordered URL list; each URL is retried `MaxRetries` times before falling through to the next one, for both the probe and the actual segment downloads.
- [ ] FTP support — **not done**. Deferred rather than rushed: a real FTP client needs to handle passive/active mode, control-connection state, and encoding quirks that make hand-rolling it a worse trade than the WebSocket case (which was a narrow, well-specified protocol, cheap to verify). Plan: use an existing, well-tested Go FTP client library (e.g. `github.com/jlaffaye/ftp`) rather than writing one from scratch — same reasoning as the Mega.nz decision below.
- [x] **Verified, not just compiled**: 8 unit tests against local `httptest` servers (segmented download correctness, resume-from-saved-offset with an assertion on the actual `Range` header sent, checksum match/mismatch, mirror fallback, retry-then-succeed, pause-mid-transfer-then-resume) — all pass, including 30 repeated runs of the scheduler package to rule out flakiness. Also ran a **live end-to-end test**: real server, real local file server, `POST /api/v1/tasks` over HTTP, polled until `complete`, verified the on-disk file's SHA-256 matched. Two real bugs were caught this way and fixed (see Notes).
- [x] Wired into the running system, closing a gap from Phase 1: `POST /api/v1/tasks` (`core/internal/api/addtask.go`) actually creates and starts a download — Phase 1 had list/pause/resume/cancel/remove but no way to *add* a task at all.

**Notes on bugs the live/repeated testing caught (all fixed):**
- `httpengine` never created its destination directory — `os.OpenFile(..., O_CREATE, ...)` doesn't create missing parent dirs, so the very first live test failed immediately. Fixed: `run()` now `os.MkdirAll`s the destination directory upfront.
- The scheduler's on-disk snapshot only updated when something called a `Scheduler` method — a task finishing on its own in the background (the normal case) never got persisted, so `tasks.json` could go stale. Fixed: added `Scheduler.PersistPeriodically(ctx, interval)`, run as a background goroutine from `main.go`.
- `Record.Error` didn't exist — a failed task's `status` was visible but not *why*. Added an optional `Err() error` capability that engines can implement (httpengine does) and that the scheduler surfaces in `Record.Error` when present.
- The scheduler's priority-then-age sort had no tiebreaker for two tasks enqueued close enough together to get an identical timestamp — ties fell back to Go's randomized map iteration order via an unstable sort, making "which task runs" flip non-deterministically between runs of the exact same test. Fixed by adding a deterministic task-ID tiebreak; verified with 30 repeated test runs.

### File-hosting / one-click-hoster support (Mediafire, Dropbox, Google Drive, Mega.nz, Pixeldrain, Catbox, Gofile, +more)

Built one **Resolver plugin interface** (`core/internal/resolver`: `CanResolve`/`Resolve`, dispatched via a `Registry`) the same way engines plug into the `Task` interface, so more hosts can be added later without touching the core — mirrors how yt-dlp grows its extractor list one site at a time. Wired into `POST /api/v1/tasks`: a submitted URL is run through the registry first, falling through to a direct fetch if no resolver claims it.

Difficulty varies a lot by site — being honest about that up front:

- [x] **Dropbox** (easy) — `core/internal/resolver/dropbox.go`: rewrites the `dl` query param to `1` (adding it if absent). Unit-tested.
- [x] **Pixeldrain** (easy — confirmed against its real docs, and its live API) — `core/internal/resolver/pixeldrain.go`: extracts the file ID from a `/u/{id}` share link and rewrites to `https://pixeldrain.com/api/file/{id}`, which returns the raw file directly, no auth, with byte-range support (pairs well with the segmented engine above). Unit-tested, and live-checked with a real request against the actual API (confirmed reachable, returns a proper 404 for an unknown ID).
- [ ] **Catbox.moe** (trivial) — not yet implemented, but genuinely needs no resolver code at all: uploaded files already get a permanent, static, direct URL (`files.catbox.moe/xxxx.ext`), so it Just Works once submitted as a plain URL. Note: prohibits `.exe`/`.doc*`/`.jar` and a few other extensions, and disallows commercial hotlinking per their FAQ.
- [ ] **Google Drive** (moderate) — not yet implemented. Public file links need the "can't scan this file for viruses, download anyway?" confirmation flow handled (an extra token/cookie exchange), plus detecting folder vs. single-file links. Well-documented pattern (see how tools like `gdown` do it), but fragile to Google changing the flow without notice.
- [ ] **MediaFire** (moderate, fragile) — not yet implemented. No public API for arbitrary files; requires scraping the share page's HTML for the real CDN download link, same category of problem as a yt-dlp site extractor. Expect to have to fix this resolver periodically when MediaFire changes their markup.
- [ ] **Gofile.io** (candidate, needs confirmation) — not yet implemented. Reportedly has a documented public API for direct links, but this wasn't independently verifiable via a plain fetch when researched (likely a JS-rendered docs site) — confirm the actual API surface before committing to it as a resolver target.
- [ ] **Mega.nz** (hard — different category of problem, not yet implemented) — Mega encrypts everything client-side (AES-128 CTR) with the decryption key embedded in the URL fragment (`#key`); this isn't a scraping problem, it's a full crypto/API client. **Decision: use an existing, well-tested Go MEGA client library (e.g. `t3rm1n4l/go-mega`, or study rclone's `mega` backend, which is a hardened fork of it) rather than hand-rolling MEGA's encryption scheme.** Unlike the WebSocket work in Phase 1 — a well-specified protocol that was cheap to verify against a real client — getting crypto subtly wrong here fails silently (corrupted files) or worse, so this is a case where reuse is clearly the right call, not hand-rolling for the sake of avoiding a dependency. Now that a real Go toolchain is available (see Phase 1), the blocker is just doing the integration work, not the earlier "can't verify a new dependency" concern.
- [x] **Workupload** — investigated and **deprioritized**: every page fetched (including their FAQ) returned a Cloudflare-style "checking that you are not a robot" challenge instead of content, and no public API was discoverable. Building a resolver would mean solving a bot-check/JS challenge rather than calling a documented endpoint — fragile, and edges toward circumventing their anti-automation protection rather than just fetching a file. Revisit only if they publish a real API later.
- [x] Note in docs: several one-click hosters' ToS restrict automated/bulk downloading — added to README.

## Phase 3 — BitTorrent Engine + Privacy Layer

- [ ] Integrate chosen BitTorrent library; support `.torrent` file and magnet link input
- [ ] DHT, PEX, tracker support
- [ ] Per-task and global proxy binding: SOCKS5 first, then evaluate WireGuard-native binding
- [ ] **Kill switch**: detect proxy/VPN disconnect and auto-pause all torrent tasks instead of leaking real IP
- [ ] **IP-leak self-check**: compare public IP with/without proxy before a torrent task starts; surface result in the UI
- [ ] Seeding controls (ratio limits, seed time limits)
- [ ] Bandwidth limits specific to torrent tasks
- [ ] Document recommended Docker network isolation pattern (sidecar VPN container via `network_mode: service:vpn`)
- [ ] Manual test: confirm no real-IP leak occurs under simulated proxy drop
- [x] Decide VPN provider support strategy → **no per-provider code in NekoDL.** Confirmed by reading gluetun's source (`internal/provider/`) that it natively supports ~25 named providers (ProtonVPN, Mullvad, PIA, AirVPN, IVPN, Windscribe, Surfshark, NordVPN, CyberGhost, ExpressVPN, etc. — auth via env vars) plus a generic "custom" WireGuard/OpenVPN config mode for everything else. NekoDL's job is just the per-task proxy/WireGuard binding + kill switch above; gluetun (or any standard WireGuard/OpenVPN config) does provider auth.
- [x] Explicitly out of scope: VPNs using proprietary, non-standard protocols with no public client library (e.g. Hotspot Shield's Catapult Hydra) — no way to integrate without reverse-engineering, and Hotspot Shield's free tier bans P2P anyway.
- [x] Explicitly not a target use case: "free" VPN services. Most prohibit P2P/torrenting in their ToS (ProtonVPN Free, Windscribe Free, TunnelBear); NekoDL won't maintain a "recommended free VPN" list.

## Phase 4 — yt-dlp Engine ("fixed yt-dlp")

- [ ] Bundle/pin a specific yt-dlp release in the Docker image
- [ ] Auto-update mechanism for yt-dlp binary (checked on a schedule, not silently mid-download)
- [ ] Maintain NekoDL's patch set for upstream bugs/site breakage not yet merged — document the patching workflow (fork + cherry-pick vs local patch files)
- [ ] Wrap yt-dlp as a managed subprocess: parse its progress output into NekoDL's task progress model
- [ ] Support common yt-dlp options via the UI/API: format selection, playlist handling, subtitles, output templates
- [ ] Route yt-dlp downloads through a proxy when explicitly configured per-task (off by default, unlike torrents)
- [ ] Cookie import support (for sites requiring login, same UX pattern as the Booth engine)

### YouTube subscriptions (Youtarr-inspired)

Absorb [Youtarr](https://github.com/DialmasterOrg/Youtarr)'s subscription-manager feature set into NekoDL's own yt-dlp engine rather than running Youtarr as a separate service (see Open Questions if a sidecar mode is wanted instead).

- [ ] Channel and playlist subscription model (subscribe once, track known videos to avoid re-downloading)
- [ ] Cron-based scheduler for auto-downloading new uploads, including Shorts/streams, with per-channel on/off toggles per content type
- [ ] SponsorBlock integration (segment removal via yt-dlp's built-in SponsorBlock support)
- [ ] Per-channel quality/resolution settings, plus a global default
- [ ] Channel grouping into subfolders (e.g. `__kids`, `__music`) for multi-library media server setups
- [ ] Metadata generation: NFO files, poster/thumbnail images, embedded metadata — matching what Plex/Jellyfin/Emby/Kodi expect
- [ ] Download history + duplicate prevention
- [ ] Age/space-based auto-cleanup with dry-run preview before deleting anything
- [ ] Library-refresh call to Plex/Jellyfin/Emby after new videos are downloaded
- [ ] Optional webhook notifications (e.g. Discord) on new downloads
- [ ] Filesystem rescan: reconcile NekoDL's database if files are moved/renamed/converted outside the app

## Phase 5 — BoothDownloader Engine

- [ ] Decide integration approach (see Phase 0 decision) and implement the wrapper/reimplementation
- [ ] Support all BoothDownloader input formats: item URL, item ID, `gifts`, `orders`, `owned`
- [ ] Cookie/access-token input + secure storage (this token grants access to a real account — treat like a secret, never log it)
- [ ] AutoZip / output folder structure config, mirroring BoothDownloader's own config options
- [ ] Surface Booth-specific errors (e.g. item not owned, token expired) clearly in the UI
- [ ] Credit Myrkie/BoothDownloader prominently in-app, not just in docs (Apache-2.0 requires attribution)

## Phase 6 — Plex Ripper Engine (Pledo-inspired)

Native engine modeled on [nekosuneprojects/pledo](https://github.com/nekosuneprojects/pledo) for downloading directly from a Plex server you have streaming access to (your own, or a shared one).

- [ ] Implement plex.tv OAuth login flow — never ask for or store a Plex password
- [ ] Sync accessible Plex servers + their library metadata into NekoDL's local storage
- [ ] Background sync job to track servers going online/offline or changing address
- [ ] Browse UI: libraries → movies / TV shows / seasons / episodes / playlists
- [ ] Support downloading multiple file versions per item (resolution/codec) when available
- [ ] Support both download modes: server-side fetch (direct file access) and in-browser stream capture, matching Pledo's approach
- [ ] Download history view (running/pending/finished/cancelled), clearable independent of the main task queue if needed
- [ ] Handle multi-episode/season batch downloads as a single grouped task in the UI

## Phase 7 — Web GUI Dashboard

- [x] Scaffold SPA — Vite + React + TypeScript in `web/`, built with `npm create vite@latest -- --template react-ts`. Build-verified: `npm run build` and `npm run lint` both pass.
- [x] Design system setup — Tailwind CSS v4 (`@tailwindcss/vite` plugin, CSS-first `@theme` config) with dark surface tokens and a green `brand` scale (`web/src/index.css`)
- [x] Build custom **Toast** component — success/error/warning/info variants, auto-dismiss (5s) + manual dismiss, stacking (`web/src/components/Toast.tsx`)
- [x] Build custom **Modal/Dialog** component — focus-trapped, escape-to-close, keyboard-accessible (`web/src/components/Modal.tsx`)
- [x] Hard rule: no `window.alert()` / `window.confirm()` / `window.prompt()` — enforced via oxlint's `no-alert` rule in `web/.oxlintrc.json`, verified it actually fires on a test file before removing the test
- [ ] Shared component kit: buttons, inputs, dropdowns, progress bars, badges, tabs — only ad-hoc Tailwind classes on individual buttons exist so far, not an extracted, reusable kit
- [ ] Task list view: status, speed, ETA, progress bar, engine type icon (HTTP/torrent/yt-dlp/Booth)
- [ ] Add-task flow: URL/magnet/torrent-file/Booth-ID input, auto-detect engine type
- [ ] Per-task detail view: files, peers (for torrents), logs (for yt-dlp/Booth)
- [ ] Global + per-task settings: bandwidth limits, proxy/VPN selection, download directory
- [ ] Live updates via WebSocket (no polling)
- [ ] Mobile-friendly layout
- [ ] Auth screen (login with API token) — using the custom modal/form components, not browser-native prompts

## Phase 8 — Docker & Deployment

- [ ] Multi-stage Dockerfile (build core + web, ship a slim runtime image)
- [ ] `docker-compose.yml` — standalone mode
- [ ] `docker-compose.vpn.yml` (or profile) — example compose wiring NekoDL's torrent engine through a gluetun/WireGuard sidecar
- [ ] Persistent volumes: downloads dir, config, session state
- [ ] Health checks
- [ ] Publish images (GHCR or Docker Hub — decide which)
- [ ] Document environment variables / config precedence

## Phase 9 — *arr Suite Integration (Radarr / Sonarr / Lidarr / Prowlarr / Ombi)

Radarr, Sonarr, and Lidarr all natively support **Aria2** as a download client type (JSON-RPC + secret token). Implementing the aria2-compatible RPC shim (Phase 12) is therefore the single integration point that unlocks all three — no per-app client code should be needed. Prowlarr does not connect to download clients directly; it syncs trackers/indexers into Radarr/Sonarr/Lidarr, which then hand accepted releases to whichever download client is configured (i.e. NekoDL). Ombi sits in front of all of it as the user-facing request tool, talking to Radarr/Sonarr/Lidarr directly — it never talks to NekoDL.

- [ ] Confirm exact RPC calls Radarr/Sonarr/Lidarr issue against an "Aria2" download client (`addUri`, `addTorrent`, `tellStatus`, `getFiles`, `remove`, `getGlobalStat`) by testing against real instances
- [ ] Treat the aria2 RPC shim (Phase 12) as a hard prerequisite for this phase — pull it forward if needed
- [ ] Support category/label → download directory mapping, since Radarr/Sonarr/Lidarr each pass a category (e.g. `movies`, `tv`, `music`) that should route to the right root folder for import
- [ ] Ensure completed-task file listings report full, correct file paths (used by each app to import/rename/hardlink into its library)
- [ ] Scope v1 to torrents only — Radarr/Sonarr/Lidarr also support Usenet clients (SABnzbd/NZBGet); NZB/Usenet support is out of scope unless a future NNTP engine is added (see Open Questions)
- [ ] Document the intended setup flow in README: Prowlarr → indexer sync → Radarr/Sonarr/Lidarr → NekoDL (as download client)
- [ ] Manual end-to-end test: add NekoDL as a download client in Radarr, grab a movie via a Prowlarr-synced indexer, confirm download + import
- [ ] Repeat the manual end-to-end test for Sonarr (TV) and Lidarr (music)
- [ ] Tag *arr-originated tasks in the Web GUI with their source app/category so they're visually distinct from manually added downloads
- [ ] Support pause/resume and queue-position/priority changes via RPC (`aria2.pause`, `aria2.changePosition`) — Radarr/Sonarr/Lidarr use these to reorder/manage the queue, not just add/remove
- [ ] Handle multi-file torrents correctly for TV season packs and multi-disc albums (per-file status, not just per-task)

### Ombi (request front end)

No direct NekoDL↔Ombi API integration is planned — Ombi already gets everything it needs from Radarr/Sonarr/Lidarr and from Plex/Emby/Jellyfin. This is just about confirming the chain works end-to-end.

- [ ] Document the full chain in README/setup docs: Ombi (request) → Radarr/Sonarr/Lidarr (find, via Prowlarr indexers) → NekoDL (fetch) → Tdarr (transcode) → Plex/Jellyfin (available)
- [ ] Manual end-to-end test: submit a request in Ombi, confirm it reaches Radarr, gets grabbed, downloads via NekoDL, and Ombi correctly marks it "available" once imported
- [ ] Revisit if Ombi ever adds a direct download-client integration mode that NekoDL should implement against

## Phase 10 — Tdarr Transcode Integration

[Tdarr](https://home.tdarr.io/) is a separate concern from acquisition (*arr) and fetching (NekoDL's own engines): it's a distributed FFmpeg/HandBrake-based transcode automation tool (Server + Node workers) that watches media library folders and runs conditional plugin "Flows" against files already on disk. NekoDL does not reimplement any transcoding — it integrates with an existing Tdarr deployment.

- [ ] Confirm scope boundary in docs: NekoDL fetches/imports media; Tdarr transcodes it afterward. No FFmpeg logic lives in NekoDL itself.
- [ ] Document the shared-volume requirement: NekoDL's completed-download and *arr-import directories must be the same paths Tdarr's folder watcher is configured to scan (Docker volume layout)
- [ ] Baseline integration: rely on Tdarr's own folder watcher/scheduled scan to pick up new files — no NekoDL-side code required beyond correct volume mounts
- [ ] Optional "notify Tdarr" hook: on task completion (especially *arr-sourced tasks), call the Tdarr Server API to trigger an immediate scan of the affected file/folder instead of waiting for Tdarr's scheduled scan
- [ ] Confirm current Tdarr Server API surface/version for triggering scans or queueing specific files
- [ ] Config screen: point NekoDL at a Tdarr Server URL (+ API key if required), global or per-library
- [ ] Optional: read-only Tdarr queue/health panel in the NekoDL Web GUI (pulled from Tdarr Server API) for a single-pane view across acquisition, download, and transcode
- [ ] Document a full example Docker Compose stack: NekoDL + Radarr/Sonarr/Lidarr/Prowlarr + Tdarr Server/Node sharing the same media volumes
- [ ] Manual test: complete a Radarr-triggered download in NekoDL, confirm Tdarr picks it up (watcher or notify hook) and transcodes per its configured Flow

## Phase 11 — NekoDL Channels (Tunarr-inspired Live TV)

A from-scratch, NekoDL-native equivalent of [Tunarr](https://tunarr.com/): scheduled live "TV channels" built from an existing Plex or Jellyfin library. This is a standalone subsystem, not a download engine — it doesn't fit the `Task` interface from Phase 1, since it schedules/streams existing media rather than fetching new media.

- [ ] Media source connectors: Plex and Jellyfin library browsing/search (Emby is a possible future add — see Open Questions)
- [ ] Channel data model: ordered lineup, filler content slots (commercials/bumpers/branding), per-channel logo
- [ ] Drag-and-drop lineup editor in the Web GUI
- [ ] Scheduling engine: time-slot scheduling and random-slot scheduling
- [ ] Web-based guide view for browsing channel lineups (XMLTV-style data)
- [ ] Playback output: M3U/IPTV endpoint for external players (Tivimate, UHF, Dispatcharr, etc.)
- [ ] Spoofed HDHomeRun tuner endpoint so Plex/Jellyfin can auto-discover NekoDL Channels as a native Live TV source
- [ ] In-browser channel preview playback
- [ ] On-the-fly transcoding for playback; evaluate hardware acceleration (NVENC/VAAPI/QuickSync) scope for v1 vs. later
- [ ] Per-channel audio language/subtitle preferences
- [ ] Automatic configuration backups for channel lineups
- [ ] Manual test: build a channel from a Plex library, tune in via the spoofed HDHR endpoint from within Plex

## Phase 12 — Compatibility & Extensibility

- [ ] Implement an aria2-compatible JSON-RPC shim (`aria2.addUri`, `aria2.addTorrent`, `aria2.tellStatus`, `aria2.getFiles`, `aria2.remove`, `aria2.getGlobalStat`, etc.) so existing aria2 clients — including Radarr/Sonarr/Lidarr's built-in Aria2 download client support (Phase 9) — work against NekoDL unmodified
- [ ] Formalize the engine-plugin interface so future site-specific downloaders can be added without touching core (candidates: Fanbox, Fantia, Gumroad, Patreon)
- [ ] CLI companion tool for headless/scripted use

## Phase 13 — Polish, Docs, Release

- [x] Finalize license and add LICENSE file (MIT, added in Phase 0)
- [ ] Write user-facing setup docs (replacing the "Quick Start" placeholder in README.md)
- [ ] Write CONTRIBUTING.md
- [ ] Security review of secret handling (Booth token, proxy credentials, API tokens, aria2/*arr RPC secret)
- [ ] Tag v0.1.0 and update CHANGELOG.md
- [ ] Set up CI (build, lint, test) and container image publishing

---

## Open Questions

Resolved in Phase 0 (license, core language, BoothDownloader invocation, config format, repo layout — see Phase 0 above). Still open:

- Should the aria2-compatible RPC shim (needed for Radarr/Sonarr/Lidarr, Phase 9/12) be pulled forward and built earlier than Phase 12, given how much it unlocks?
- Is Usenet/NZB support (SABnzbd/NZBGet-style) ever in scope, or is NekoDL torrent/yt-dlp/Booth only? Affects how much *arr-stack coverage is achievable.
- For *arr category→directory mapping: fixed config mapping, or auto-create subfolders per category on demand?
- Is the Tdarr "notify" API hook (Phase 10) worth building for v1, or is folder-watcher-only (no direct NekoDL→Tdarr API call) sufficient for launch?
- Plex ripper engine: does downloading from a friend's shared Plex server (not just your own) raise any ToS/legal concerns worth documenting explicitly for users?
- YouTube subscriptions: fully absorb Youtarr's feature set into NekoDL's own yt-dlp engine (current plan), or also support running Youtarr itself as an optional sidecar for users who already have it set up?
- NekoDL Channels: is Emby parity with Tunarr worth adding, or is Plex + Jellyfin sufficient for v1?
- NekoDL Channels: how much hardware-transcode parity with Tunarr (NVENC/VAAPI/QuickSync) is needed for v1 vs. deferred to a later release?
