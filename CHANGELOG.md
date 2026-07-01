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
- Scaffolded the Go core skeleton (`core/`): JSON config loader, stdlib HTTP server with a `/health` endpoint, graceful shutdown, and the internal `Task` interface that every download engine will implement. Not yet build-verified — no Go toolchain was available in this environment to run `go build`/`go vet`.
- Scaffolded the web dashboard (`web/`): Vite + React + TypeScript + Tailwind CSS v4, dark/green design tokens, and working `ToastProvider`/`Toast` and `Modal` components with a `no-alert` lint rule enforcing the "no native browser dialogs" constraint. Build-verified with `npm run build` and `npm run lint`.

### Notes
- Beyond the two scaffolds above, no download engines, scheduler, task queue, or real dashboard views exist yet — see [TODO.md](TODO.md) for the phased build plan.

[Unreleased]: https://github.com/NekoSuneVR/NekoDL
