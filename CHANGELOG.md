# Changelog

All notable changes to NekoDL will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project will adhere to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) once the first release ships.

## [Unreleased]

### Added
- Initial project planning: `README.md`, `TODO.md`, and this `CHANGELOG.md`.
- Proposed architecture: Go core with pluggable engines (HTTP/FTP, BitTorrent via `anacrolix/torrent`, yt-dlp as a managed subprocess, BoothDownloader wrapper), React web dashboard, Docker-first deployment.
- Defined privacy model for torrenting: per-task SOCKS5/WireGuard proxy binding, kill switch on VPN/proxy drop, pre-flight IP-leak self-check.
- Planned *arr suite integration: NekoDL as a drop-in download client for Radarr/Sonarr/Lidarr via an aria2-compatible JSON-RPC shim, fed by Prowlarr-synced indexers.
- Planned Tdarr transcode integration: shared media volume with an existing Tdarr Server/Node deployment, plus an optional "scan now" notify hook on task completion. NekoDL does not perform any transcoding itself.
- Planned UI direction: Tailwind CSS, dark/green design system, fully custom toast and modal components — no native `alert()`/`confirm()`/`prompt()` anywhere in the app.
- Planned YouTube channel/playlist subscriptions for the yt-dlp engine, absorbing Youtarr's feature set (scheduled auto-download, SponsorBlock, NFO/poster metadata, auto-cleanup, media-server library refresh).
- Planned "NekoDL Channels": a from-scratch, Tunarr-inspired live-TV module that schedules and streams existing Plex/Jellyfin library content as virtual channels (M3U + spoofed HDHomeRun tuner output).
- Planned Plex ripping engine, inspired by [Pledo](https://github.com/nekosuneprojects/pledo): download movies/TV/playlists directly from an accessible Plex server via plex.tv login.
- Documented the Ombi request flow: Ombi → Radarr/Sonarr/Lidarr → NekoDL → Tdarr → Plex/Jellyfin, with no direct NekoDL↔Ombi API integration planned (Ombi already talks to the *arr apps and media servers directly).
- Recorded Phase 0 decisions: MIT license, Go core, JSON config (`nekodl.json`) for v1, monorepo layout (`core/`, `web/`, `docker/`), BoothDownloader invoked by shelling out to its CLI.
- Added `LICENSE` (MIT) and `.gitignore`.
- Scaffolded the Go core skeleton (`core/`): JSON config loader, stdlib HTTP server with a `/health` endpoint, graceful shutdown, and the internal `Task` interface that every download engine will implement.
- Scaffolded the web dashboard (`web/`): Vite + React + TypeScript + Tailwind CSS v4, dark/green design tokens, and working `ToastProvider`/`Toast` and `Modal` components with a `no-alert` lint rule enforcing the "no native browser dialogs" constraint. Build-verified with `npm run build` and `npm run lint`.

- Decided VPN provider support strategy: no per-provider integration code in NekoDL. Named providers (ProtonVPN, Mullvad, PIA, AirVPN, IVPN, Windscribe, Surfshark, NordVPN, CyberGhost, ExpressVPN, etc.) work via gluetun's native support; any other provider works via a standard WireGuard/OpenVPN config. Proprietary-protocol VPNs (e.g. Hotspot Shield) and "free" VPN services are explicitly out of scope.
- Completed the rest of Phase 1 (core skeleton): a task queue/scheduler (`core/internal/scheduler`) with a global concurrency limit and priority ordering, JSON snapshot persistence of task metadata, a REST API for task CRUD using Go 1.22's stdlib method+wildcard routing, bearer-token auth with constant-time comparison, and unit tests covering scheduler/task lifecycle behavior.
- Installed a Go 1.26 toolchain (via `winget`) and used it to actually verify everything above: `go build ./...` and `go vet ./...` pass clean, and all 7 scheduler/store unit tests pass.
- Real WebSocket support for live progress events (`GET /api/v1/events`), hand-rolled against RFC 6455 using only the Go standard library — replacing the earlier Server-Sent Events placeholder. Verified live by running the server and connecting a genuine, independent WebSocket client to it (not just a compile check): the handshake completed and 5 correctly-framed JSON text frames were received before a clean disconnect.
- Planned one-click-hoster support (Mediafire, Dropbox, Google Drive, Mega.nz) in Phase 2 via a pluggable Resolver interface. Decided to use an existing Go MEGA client library rather than hand-rolling Mega.nz's client-side encryption scheme.
- Researched and added Pixeldrain (confirmed real public API, no scraping) and Catbox.moe (trivial — files are already static direct URLs) as easy Phase 2 resolver targets; added Gofile as an unconfirmed candidate. Evaluated Workupload and deprioritized it — every page checked was behind a Cloudflare bot-check with no discoverable API.
- Built the HTTP download engine (`core/internal/httpengine`): segmented multi-connection downloading with byte-range probing, resume via a JSON progress sidecar file, checksum verification (MD5/SHA-1/SHA-256), per-request retry/backoff, and mirror/fallback URLs. Covered by 8 unit tests against local `httptest` servers, all passing, plus a real live end-to-end test (actual server, actual local file server, real HTTP requests, SHA-256-verified on-disk result).
- Built Dropbox and Pixeldrain resolvers (`core/internal/resolver`) behind a pluggable `Resolver`/`Registry` interface; unit-tested, and Pixeldrain's live API endpoint independently confirmed reachable.
- Wired the HTTP engine and resolvers into the API: `POST /api/v1/tasks` now actually creates and starts a download (Phase 1 had list/pause/resume/cancel/remove but no way to add a task at all).
- Fixed three real bugs surfaced by live/repeated testing rather than by inspection: (1) the destination directory was never created before writing, failing every real download immediately; (2) the scheduler's on-disk snapshot never updated when a task finished on its own in the background — added `Scheduler.PersistPeriodically`; (3) the scheduler's priority/age sort had no tiebreaker for same-timestamp tasks, so which one ran could flip non-deterministically between runs due to Go's randomized map iteration feeding an unstable sort — fixed with a deterministic ID tiebreak, confirmed with 30 repeated test runs.
- Added `Record.Error`, surfacing *why* a task failed (previously only `status: error` was visible, with no reason) via an optional `Err() error` capability engines can implement.
- Built Google Drive and Mediafire resolvers. Google Drive's ID-extraction/direct-download-URL rewrite is confirmed against Google's real server (a genuine 303 redirect to `drive.usercontent.google.com`); the large-file "confirm download anyway" interstitial is explicitly not implemented (no Google account available to verify it). Mediafire scrapes the share page's `downloadButton` link; tested against constructed mock HTML since every real Mediafire link found during research was malware-flagged and thus not used as a test fixture.
- Confirmed, with two independent live checks (a fresh guest token and the uploading account's own token against its own file), that Gofile.io's API requires a **paid account** to resolve download links — not implemented, and not a "maybe later."
- Implemented Mega.nz support as its own package (`core/internal/megalink`), not a `resolver.Resolver` — its temp URL serves ciphertext, so decryption has to happen while streaming, not via a URL rewrite. Revisited the earlier "use an existing library" decision after actually checking the options: the one well-maintained Go MEGA library has no public-link download support at all (confirmed by reading its source); the libraries that do support public links haven't been updated since 2019–2021. Implemented the key-derivation/CTR-decryption/attribute-decryption scheme instead, transcribed precisely from a long-standing reference implementation (`juanriaza/python-mega`) rather than from memory, using only stdlib `crypto/aes`/`crypto/cipher` for the actual cipher. 11 unit tests, including a full pipeline test that independently encrypts real data with Go's own stdlib and confirms this package decrypts it correctly. Not verified: the real MEGA API's current request/response shape, and MAC/integrity verification isn't implemented. Not yet wired into `POST /api/v1/tasks`.

### Notes
- FTP support is still not implemented — see [TODO.md](TODO.md) Phase 2.
- `megalink.Downloader` (Mega.nz) exists but isn't reachable from the API yet.
- No BitTorrent, yt-dlp, Booth, or Plex-ripper engines exist yet.

[Unreleased]: https://github.com/NekoSuneVR/NekoDL
