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
- [x] **Catbox.moe** (trivial) — confirmed live: real anonymous upload → real 206 Partial Content with a correct `Content-Range` header on a `Range` request. Genuinely needs no resolver code — a plain Catbox URL already works as-is through the HTTP engine, no code to write or maintain.
- [x] **Google Drive** (moderate) — `core/internal/resolver/googledrive.go`: extracts the file ID from `/file/d/{id}/...` or `?id={id}` links and rewrites to `https://drive.google.com/uc?export=download&id={id}`. The redirect mechanics are confirmed live (a real request to Google's real server returned an actual 303 to `drive.usercontent.google.com/download?...`, which `http.Client` follows automatically) — but the specific test file ID used no longer resolves to real content, so this covers the small-file happy path with real confidence, not a full live download. **Explicitly not implemented**: the "can't scan this file for viruses, download anyway?" confirm-token interstitial for large files — no Google account/browser was available to trigger and verify that flow, so it's left undone rather than guessed at.
- [x] **MediaFire** (moderate, fragile) — `core/internal/resolver/mediafire.go`: scrapes the share page for `<a id="downloadButton" href="...">`, tolerant of attribute order and HTML entity escaping. **Not live-verified** — every real Mediafire link found during research was malware-flagged (a "cracking tool" and a modded APK; deliberately not used as test fixtures), and Mediafire requires an account for uploads so no safe test file could be created. Tested against constructed mock HTML matching the documented pattern instead.
- [x] **Gofile.io** — investigated and **deprioritized with a confirmed reason**, not just "unclear": its API is real and reachable (`POST api.gofile.io/accounts` issues a working guest token, uploads succeed), but resolving content via `GET /contents/{id}` returns `error-notPremium` for **both** a fresh independent guest token and the uploading account's own token, on its own freshly-uploaded file. Confirmed twice. Gofile's free tier cannot resolve download links via the API at all — this isn't a "maybe later" gap, it's a paid-account requirement that doesn't fit a free general-purpose resolver.
- [x] **Mega.nz** (hard — different category of problem) — implemented in a new package, `core/internal/megalink`, *not* as a `resolver.Resolver`: Mega's temporary URL serves ciphertext, so decryption has to happen while streaming the download, not before it as a URL rewrite. Revisited the original "use an existing library" decision after checking the actual options: the one actively-maintained Go MEGA library (`t3rm1n4l/go-mega`, updated the day before this was written) has **no function for downloading from someone else's public link at all** — confirmed by reading its source, every operation requires an authenticated account browsing its own filesystem tree. The libraries that *do* claim public-link support (`u3mur4/megadl`, `mocukie/megalink`, others) haven't been touched since 2019–2021 — using one would trade "risk of my own fresh bug" for "risk of someone else's stale, unverifiable-against-current-MEGA bug," which isn't clearly better. Implemented the key-derivation/CTR-decryption/attribute-decryption scheme instead, precisely transcribed from `juanriaza/python-mega` (a long-standing, widely-used reference implementation) rather than from memory or reverse-engineering from scratch — and it only uses stdlib `crypto/aes`/`crypto/cipher` for the actual cipher, never hand-rolled crypto primitives. **Verified**: 11 unit tests, including a full pipeline test that independently AES-CTR-encrypts real bytes and AES-CBC-encrypts real attributes with Go's own stdlib (acting as an "independent encryptor"), feeds them through two mock HTTP servers standing in for MEGA's API and CDN, and confirms the exact original plaintext and filename come back out. **Not verified**: whether the real, live `g.api.mega.co.nz` API still matches this exact request/response shape today — no MEGA account or safe public test file was available to check. Also not implemented: MAC/integrity verification (`FileKey.MetaMAC` is derived but unused — a corrupted download isn't currently detected), and it's not yet wired into `POST /api/v1/tasks` (that endpoint's "resolve to a URL, hand to httpengine" design doesn't fit something that needs post-download decryption — needs its own code path).
- [x] **Workupload** — investigated and **deprioritized**: every page fetched (including their FAQ) returned a Cloudflare-style "checking that you are not a robot" challenge instead of content, and no public API was discoverable. Building a resolver would mean solving a bot-check/JS challenge rather than calling a documented endpoint — fragile, and edges toward circumventing their anti-automation protection rather than just fetching a file. Revisit only if they publish a real API later.
- [x] Note in docs: several one-click hosters' ToS restrict automated/bulk downloading — added to README.
- [x] Wire `megalink.Downloader` into `POST /api/v1/tasks` — added `megalink.Task` (`core/internal/megalink/task.go`), a `task.Task` implementation wrapping `Downloader`. `handleAddTask` now checks `megalink.CanResolve(url)` before the generic resolver path and dispatches to a MEGA task when it matches. Single-shot only: Pause() stops the transfer but Resume() after a pause restarts from scratch rather than continuing, since MEGA's temp URLs weren't confirmed to support resumable `Range` requests — a real, documented limitation, not an oversight. Verified with 3 more tests (full lifecycle to completion with correct filename/byte count, failure path, cancel), on top of the 11 from the Downloader itself.
- [~] Verify the Google Drive large-file confirm-token flow and the MediaFire/Mega.nz live API assumptions against real accounts/files — tried again (searched for a real, currently-valid large public Google Drive test file); every source describing the confirm-token bypass uses a placeholder ID, none pointed to a real, currently-live file. Confirms the *mechanism* (confirm-token redirect) is real and current, but there's still no way to test it end-to-end without a Google account. Stays open, blocked on access rather than effort.

## Phase 3 — BitTorrent Engine + Privacy Layer

- [x] Integrate chosen BitTorrent library; support `.torrent` file and magnet link input — `core/internal/torrentengine`, real `github.com/anacrolix/torrent` dependency (actively maintained, updated the day before this was added; 6050 stars). `Task.addTorrent` dispatches to `client.AddMagnet` or `client.AddTorrent` (via `metainfo.Load` on the uploaded bytes) depending on which of `Options.MagnetURI`/`TorrentBytes` is set.
- [x] DHT, PEX, tracker support — left at the library's defaults (both enabled); NekoDL only needs to not turn them off, which it doesn't unless a task explicitly requests `DisableDHT`/`DisablePEX`.
- [x] Per-task proxy binding: SOCKS5 — done. **Global** proxy binding isn't a separate concept: each `Task` owns its own `torrent.Client` (a deliberate per-task-client architecture — proxy binding is a `*Client`-level concern in this library, via `AddDialer`, not a per-`Torrent` one), so "use this proxy for everything" is just "set the same `ProxyAddr` on every task," not additional code. WireGuard-native binding: **evaluated, not implemented** — see the Open Questions/notes below for why.
- [x] **Kill switch**: detect proxy/VPN disconnect and auto-pause all torrent tasks instead of leaking real IP — implemented as `Task.runKillSwitch`, which periodically calls `DetectLeak` (below) and, on any leak, calls `DisallowDataUpload`/stops the torrent and sets `task.StatusError` (not `StatusPaused` — deliberately, so the scheduler's `rescheduleLocked` won't auto-resume a kill-switched task; it already skips `StatusError` tasks entirely).
- [x] **IP-leak self-check**: compare public IP with/without proxy — `core/internal/torrentengine/ipcheck.go`'s `DetectLeak`: fetches a public-IP echo service (`api.ipify.org` by default, injectable for tests) both directly and through the SOCKS5 proxy; reports a leak if the proxied request fails *or* returns the same IP as the direct request (a reachable-but-misconfigured proxy that isn't actually routing traffic wouldn't be caught by a simple reachability ping — comparing apparent IP catches that too). "Surface result in the UI" is still open — the check runs and drives the kill switch, but there's no dedicated API endpoint to run it on demand yet (see Open Questions).
- [x] Seeding controls (ratio limits, seed time limits) — `core/internal/torrentengine/seedlimit.go`: `Options.SeedRatioLimit`/`SeedTimeLimit`, checked every 10s via `Torrent.Stats()`'s real `BytesWrittenData`/`BytesReadData` counters. Decision logic factored into a pure function (`shouldStopSeeding`) so it's unit-testable directly with synthetic stats rather than only via a full seed/leech integration test — 6 tests covering ratio/time/unlimited/not-yet-downloaded-anything cases.
- [x] Bandwidth limits specific to torrent tasks — `Options.MaxDownloadBps`/`MaxUploadBps`, wired to `torrent.ClientConfig.DownloadRateLimiter`/`UploadRateLimiter` (both `golang.org/x/time/rate.Limiter`, the library's own expected type). Plumbing is verified (compiles, values flow through); the actual throttling behavior wasn't separately load-tested — that's exercising `x/time/rate` and the library's own internals more than NekoDL's code, so it's a reasonable place to trust the dependency rather than re-verify it.
- [x] Document recommended Docker network isolation pattern (sidecar VPN container via `network_mode: service:vpn`) — already in README.md's Privacy Model section from earlier work.
- [x] Manual test: confirm no real-IP leak occurs under simulated proxy drop — **upgraded to an automated test**, `TestKillSwitchStopsTaskWhenProxyDrops`: a real task with a real (if intentionally leak-triggering) proxy setup genuinely trips the kill switch and lands in `StatusError` with a clear `Err()`. No manual step needed to reproduce this going forward.
- [x] **Verified, not just compiled**: 12 tests total. The flagship one (`TestDownloadOverLoopback`) is a real BitTorrent transfer — an actual seeding `torrent.Client` (built from a real hashed torrent of a real random 300 KiB file) serving a real leeching `Task` over loopback TCP, introduced via `Torrent.AddClientPeer` (the library's own documented mechanism for connecting in-process test clients without a tracker/DHT). Also: pause/resume preserves swarm state (`AllowDataDownload`/`DisallowDataDownload` toggle rather than tearing down the client), a real local SOCKS5 server (`things-go/go-socks5`, test-only dependency) verifies the client-side dialer actually speaks correct SOCKS5, and the leak-detection *decision logic* is tested with synthetic IP-echo responses (a real "different exit IP" isn't reproducible on loopback, so this tests the comparison logic, not full real-world network topology).
- [x] Wired into the API: `POST /api/v1/torrents` (`core/internal/api/addtorrent.go`) — `magnet_uri` or `torrent_file_base64`, plus `proxy_addr`/rate-limit/seed options. Live-checked against the running server: an invalid request (no magnet/file) returns a clear 400; a valid magnet with no proxy returns 201 with the expected "no proxy configured" warning, which also shows up on a follow-up `GET`. Added `Record.Warning` (mirroring the existing `Record.Error` pattern) so this surfaces through the API generically via an optional `Warning() string` capability, not a torrent-specific hack.
- [x] **Risk feature**: torrents may run without a proxy — using your real IP — but never silently. `Task.Warning()` returns a clear caution when `ProxyAddr` is empty, surfaced in both the task-creation response and every subsequent `GET`.

**Notes on decisions revisited along the way:**
- The `t3rm1n4l/go-mega`-style "prefer an existing library" reasoning from the Mega.nz work does *not* transfer to WireGuard: no attempt was made to build WireGuard-native binding in this pass. SOCKS5 covers the "route torrent traffic through *a* proxy/VPN" requirement generically (any VPN client that can also expose a local SOCKS5 proxy, or any standalone SOCKS5 proxy, works); WireGuard-*native* (NekoDL directly holding a WireGuard tunnel via e.g. `golang.zx2c4.com/wireguard`) is a materially bigger undertaking — kernel/TUN device handling, key management — that wasn't attempted here. Revisit if SOCKS5-only turns out to be insufficient for real users.
- One architectural consequence worth flagging: one `torrent.Client` per `Task` means each running torrent binds its own OS port (explicitly forced to a random one in `buildClientConfig` — the library's fixed default of 42069 would otherwise make every task after the first fail to bind) and pays its own DHT-bootstrap/client-startup cost. Fine for now; a shared-client-pool-keyed-by-proxy-config design would use fewer resources for many concurrent torrents, but adds real complexity for a problem NekoDL doesn't have evidence of yet.
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
- [x] Shared component kit — extracted into `web/src/components/ui/`: `Button` (primary/secondary/danger), `Input`, `Select`, `Badge` (`StatusBadge`/`EngineBadge`), `ProgressBar`, `Tabs`. Genuinely reused across `TaskList`/`AddTaskModal`/`AuthPrompt`/`Toolbar`, not just built and abandoned — `Select` is the one exception, built for the kit but not yet consumed by any screen (no current UI need for it).
- [x] Task list view: status, speed, ETA, progress bar, engine type icon — `web/src/components/TaskList.tsx`. Real icons exist for `http`/`torrent`/`mega` (the engines that actually exist); yt-dlp/Booth icons are defined but untestable since those engines don't exist in the backend yet.
- [x] Add-task flow: URL/magnet/torrent-file auto-detect — `web/src/components/AddTaskModal.tsx`: a shared textarea (one link per line) auto-routes each line to `POST /api/v1/tasks` (URL) or `POST /api/v1/torrents` (a line starting with `magnet:`), plus a `.torrent` file picker. **Booth-ID input not implemented** — the Booth engine itself doesn't exist in the backend yet (Phase 5 unstarted), so there's no API to wire a UI to.
- [~] Per-task detail view — implemented as an expandable row (click a task) showing everything the API currently reports (id/engine/priority/added_at, plus error/warning). **Does not show files, torrent peers, or engine logs** — no backend endpoint exposes any of that yet, and the panel says so directly in the UI rather than faking it.
- [~] Global + per-task settings — per-task settings (priority, HTTP max-connections, torrent proxy/seed/bandwidth limits) implemented in the Add Download modal's "Options" tab, live-tested against the real API. **Global settings (editing the running server's own config remotely) not implemented** — no backend endpoint exists for that.
- [x] Live updates via WebSocket (no polling) — `web/src/hooks/useTasks.ts`. Building this surfaced a real gap that's now fixed: browsers' native `WebSocket` API can't set an `Authorization` header on the handshake request at all, so with an API token configured the events connection would have failed outright. Added a `?token=` query-param fallback scoped to just this one endpoint (`core/internal/api/auth.go`'s new `requireAuthWS`) — verified live end-to-end: a token-less connection gets rejected (401, never reaches the 101 upgrade) and a `?token=`-bearing one connects and streams real task data.
- [x] Mobile-friendly layout — off-canvas sidebar with a hamburger toggle below Tailwind's `md` breakpoint. Not tested on a real mobile device or browser (no such tool available here) — the responsive classes are structurally standard Tailwind, but this specific claim is weaker than the ones that were live-verified.
- [x] Auth screen — implemented as a modal (`AuthPrompt.tsx`), matching the "custom modal/form components, not browser-native prompts" requirement. Verified against the real server, including the WS auth fallback above.

**Verification note for this whole phase**: every API call the dashboard makes (`listTasks`, `addTask`, `addTorrent`, pause/resume/cancel/remove, the WebSocket events feed with and without auth) was exercised against the real, running Go server with real requests (`curl`/Monitor's `ws` source) — not just "the frontend compiles and the backend compiles separately." `npm run build` and `npm run lint` (including the `no-alert` rule) both pass on the current dashboard.

## Phase 8 — Docker & Deployment

- [x] Multi-stage Dockerfile (`Dockerfile`, repo root — build context needs both `core/` and `web/`) — stage 1 builds the web dashboard (`node:22-alpine`), stage 2 cross-compiles the Go core for the target platform (`golang:1.24-alpine`, `CGO_ENABLED=0`, using buildx's `TARGETOS`/`TARGETARCH` build args), stage 3 is a slim `alpine:3.20` runtime running as a non-root user.
- [x] Along the way: wired real static-file serving into the API (`core/internal/api/static.go`, `Config.StaticDir`) — the Dockerfile packages the built dashboard, but nothing would have served it without this. SPA-style fallback to `index.html` for unmatched paths, registered on `"/"` which Go's `ServeMux` only reaches for requests that don't match a more specific pattern, so it can't shadow the API routes. **Live-verified**: built the real dashboard, pointed a running server at it, confirmed `/` serves `index.html`, a real asset serves with the correct `Content-Type`, and `/health`/the API still work alongside it.
- [x] `docker-compose.yml` — standalone mode.
- [x] `docker-compose.vpn.yml` — gluetun sidecar example (`network_mode: "service:gluetun"`, ports published by gluetun since nekodl shares its network namespace). Not live-tested against a real VPN provider (would need real credentials) — the compose structure itself is the well-established, widely-used pattern for this (same approach as qBittorrent+gluetun setups), not something novel to verify.
- [x] Persistent volumes: `/data` (downloads + `tasks.json` session state) via a named volume in both compose files. Config: either the image's baked-in default (`nekodl.docker.json`) or a mounted override file — see the env var option below for a lighter-weight alternative.
- [x] Health checks — `HEALTHCHECK` in the Dockerfile hitting `/health` (unauthenticated by design, see Phase 1).
- [x] Publish images → **GHCR** (`ghcr.io/nekosunevr/nekodl`), via `.github/workflows/docker.yml`: multi-arch (`linux/amd64,linux/arm64`) build using `docker/setup-qemu-action` + buildx, pushed on every push to `main` and on manual `workflow_dispatch`; PRs build (catching cross-compile breakage) but don't push.
- [x] Document environment variables / config precedence — added `NEKODL_*` env var overrides to `core/internal/config` (`LISTEN_ADDR`, `DATA_DIR`, `LOG_LEVEL`, `API_TOKEN`, `STATIC_DIR`, `MAX_CONCURRENT_DOWNLOADS`), precedence env > config file > defaults, so Docker users can do `docker run -e NEKODL_API_TOKEN=...` without rebuilding or mounting a custom file. 4 unit tests (missing file → defaults, file overrides defaults, env overrides file, invalid env value is ignored rather than crashing).
- [x] Triggered and watched the workflow's first real run (`gh run watch`) — succeeded in 9m23s; confirmed by reading the actual build log (not just the green checkmark) that the pushed manifest list includes both `linux/amd64` and `linux/arm64` digests. `ghcr.io/nekosunevr/nekodl:latest` is live on GHCR right now.
- [ ] Still open: CI proves the image *builds* correctly for both architectures via QEMU emulation on an amd64 GitHub-hosted runner — nobody (including this session) has pulled and run the published image on real amd64 or real arm64 hardware yet. That's the next real-world confirmation, and needs an actual Docker host to do.

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
