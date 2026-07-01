# NekoDL

**NekoDL** is a modern, privacy-first download manager inspired by [aria2](https://github.com/aria2/aria2) — rebuilt from scratch with its own core, its own protocol, and its own Web GUI.

Where aria2 gives you HTTP/FTP/BitTorrent/Metalink, NekoDL gives you that plus **first-class yt-dlp support**, **VPN/proxy-shielded torrenting**, and a **BoothDownloader engine** for grabbing your owned Booth.pm assets — all from one dashboard, one API, and one Docker container.

> ⚠️ **Status: pre-alpha / planning stage.** This repo currently contains the project plan (see [TODO.md](TODO.md)). No code has landed yet. See [CHANGELOG.md](CHANGELOG.md) for what's actually shipped.

---

## Why NekoDL?

aria2 is fast and rock-solid, but it's frozen in time: no native yt-dlp support, no built-in VPN/proxy isolation for torrents, no modern web UI (AriaNg is a separate, third-party project), and no way to plug in site-specific downloaders like BoothDownloader without duct tape.

NekoDL keeps the parts of aria2's design that work well (a lightweight RPC-driven core, queue-based multi-connection downloading) and rebuilds the rest around three goals:

- **Privacy by default** — torrent traffic never touches your real IP unless you explicitly allow it.
- **One tool, many sources** — direct links, magnet/torrent, 1000+ yt-dlp-supported sites, and Booth.pm, all managed the same way.
- **Actually usable UI** — a real dashboard, not a bookmarklet.

## Planned Features

- 🧲 **BitTorrent engine** — DHT, PEX, magnet links, per-torrent or global SOCKS5/WireGuard proxy binding, and a **kill switch** that pauses torrent traffic if the VPN/proxy drops.
- 🎬 **yt-dlp integration** — bundled, version-pinned, auto-updating yt-dlp with NekoDL's own patch set for sites/fixes that haven't landed upstream yet ("fixed" yt-dlp).
- 📺 **YouTube channel/playlist subscriptions** — [Youtarr](https://github.com/DialmasterOrg/Youtarr)-inspired subscription layer on top of the yt-dlp engine: subscribe to channels/playlists, auto-download new uploads on a schedule, SponsorBlock segment removal, NFO/poster metadata generation, age/space-based auto-cleanup, and library-refresh triggers for Plex/Jellyfin/Emby.
- 📡 **NekoDL Channels** — a from-scratch, NekoDL-native take on [Tunarr](https://tunarr.com/): build live "TV channels" out of your Plex or Jellyfin library, schedule a lineup, and tune in via M3U/IPTV or a spoofed HDHomeRun tuner.
- 🛍️ **BoothDownloader engine** — wraps [Myrkie/BoothDownloader](https://github.com/Myrkie/BoothDownloader) to pull owned images/files/gifts/orders from [Booth.pm](https://booth.pm) using your account cookie/token.
- 🎥 **Plex ripping engine** — [Pledo](https://github.com/nekosuneprojects/pledo)-inspired: log in via plex.tv (no password typed into NekoDL), browse any Plex server you have access to, and download movies/TV/playlists directly, including multiple resolution/codec versions.
- 🌐 **HTTP/HTTPS/FTP** downloads with multi-connection segmented fetching, resume, and checksum verification.
- 🖥️ **Web GUI dashboard** — queue management, live speed/progress graphs, per-download proxy overrides, drag-and-drop `.torrent`/cookie files.
- 🎨 **Modern dark, green-themed UI** — Tailwind CSS design system, fully custom toast/alert components and modal dialogs for prompts/confirmations. No native `alert()`/`confirm()`/`prompt()` anywhere in the app.
- 🔌 **JSON-RPC / REST / WebSocket API** — scriptable like aria2, with an aria2-compatible RPC shim so existing tools (e.g. browser "send to aria2" extensions, and the *arr suite below) keep working.
- 🎞️📺🎵 ***arr* suite integration** — drop-in download client for [Radarr](https://radarr.video/), [Sonarr](https://sonarr.tv/), and [Lidarr](https://lidarr.audio/) (via the aria2-compatible RPC), fed by trackers/indexers synced from [Prowlarr](https://prowlarr.com/).
- 🎛️ **Tdarr transcode hook** — notifies an existing [Tdarr](https://home.tdarr.io/) Server when a download/*arr-import completes, so newly acquired media gets picked up for transcoding without waiting on Tdarr's own scan interval. NekoDL does not transcode anything itself.
- 🙋 **Ombi-compatible request flow** — sits at the bottom of an [Ombi](https://ombi.io/)-fronted stack: users request in Ombi, Ombi hands off to Radarr/Sonarr/Lidarr, which hand off to NekoDL over the same aria2-compatible RPC used everywhere else in the *arr suite.
- 🐳 **Docker-first** — single container, or compose with a VPN sidecar (e.g. [gluetun](https://github.com/qdm12/gluetun)) for network-level isolation.
- 🔒 **Privacy tooling** — per-task proxy/VPN selection, IP-leak self-check before a torrent starts, no telemetry.

## Architecture (proposed)

```
                          ┌─────────────────────────────┐
                          │        Web Dashboard         │
                          │   (React/Vite SPA, dark UI)  │
                          └───────────────┬──────────────┘
                                          │ REST + WebSocket
                          ┌───────────────▼──────────────┐
                          │         NekoDL Core           │
                          │  (Go — queue, scheduler, API) │
                          └───┬───────┬───────┬──────────┘
                 ┌────────────┘       │       └────────────┐
                 ▼                    ▼                    ▼
       ┌─────────────────┐ ┌───────────────────┐ ┌────────────────────┐
       │  HTTP/FTP engine │ │  BitTorrent engine │ │   yt-dlp engine    │
       │  (segmented,     │ │ (anacrolix/torrent,│ │ (managed subprocess│
       │   resumable)     │ │  SOCKS5/WG proxy,  │ │  + patch set,      │
       │                  │ │  kill switch)       │ │  + subscriptions)  │
       └─────────────────┘ └───────────────────┘ └────────────────────┘
       ┌─────────────────┐ ┌───────────────────┐
       │  Booth engine    │ │  Plex ripper engine│
       │  (BoothDownloader│ │  (Pledo-inspired,  │
       │   wrapper)       │ │   plex.tv login)   │
       └─────────────────┘ └───────────────────┘
```

The core exposes a stable API; each engine is a plugin behind a common `Task` interface (`Add`, `Pause`, `Resume`, `Cancel`, `Progress`). This is what makes it possible to add future site-specific downloaders (Fanbox, Fantia, Gumroad, etc.) without touching the core.

**Tech stack (proposed, open to revision):**

| Layer | Choice | Why |
|---|---|---|
| Core | Go | Single static binary, great concurrency, easy Docker images |
| BitTorrent | [anacrolix/torrent](https://github.com/anacrolix/torrent) | Pure Go, supports custom dialers → clean SOCKS5/WireGuard proxy binding |
| yt-dlp | Managed subprocess | yt-dlp itself stays Python; NekoDL manages the binary, config, and patches |
| Booth | Wraps BoothDownloader CLI (or reimplements its API calls) | Reuse a maintained, working implementation instead of duplicating it |
| Plex ripper | Native engine using the Plex API (plex.tv OAuth) | No external process needed; Pledo's approach is straightforward enough to reimplement directly |
| Dashboard | React + Vite + Tailwind CSS | Fast to iterate, easy to theme; utility classes keep the custom dark/green design system consistent |
| Packaging | Docker / docker-compose | Works standalone or alongside a VPN sidecar container |

See [TODO.md](TODO.md) for the full breakdown and open decisions.

## Privacy Model

Torrenting is the one protocol that leaks your IP to strangers by design, so NekoDL treats it as the sensitive path:

- Every torrent task can be bound to a **SOCKS5 proxy** or **WireGuard config**, either globally or per-task.
- A **kill switch** pauses all torrent traffic if the configured proxy/VPN connection drops, instead of silently falling back to your real IP.
- An optional **IP-leak self-check** runs before a torrent starts, comparing your public IP with and without the proxy/VPN active.
- Recommended Docker deployment: run NekoDL's torrent engine with `network_mode: "service:vpn"` alongside a VPN sidecar (e.g. [gluetun](https://github.com/qdm12/gluetun)) for OS-level network isolation, in addition to the app-level kill switch.
- HTTP/yt-dlp/Booth downloads are not routed through the torrent proxy by default (they're not P2P and don't leak your IP the same way), but per-task proxy overrides will be available for all engines.

## Media Automation (Ombi / Radarr / Sonarr / Lidarr / Prowlarr / Tdarr)

NekoDL is designed to slot into an existing [Ombi](https://ombi.io/) + [*arr](https://wiki.servarr.com/) + [Tdarr](https://home.tdarr.io/) stack, covering four distinct jobs end-to-end — **request**, **find**, **fetch**, and **transcode** — without NekoDL trying to own all of them itself:

```
Ombi (user-facing requests)          Prowlarr (indexers/trackers)
        │  creates a request in              │  syncs indexers into
        └───────────────┐      ┌─────────────┘
                         ▼      ▼
        Radarr (movies) ── Sonarr (TV) ── Lidarr (music)   ← FIND: what to grab
                          │
                          │  send accepted releases via
                          │  aria2-compatible JSON-RPC
                          ▼
                       NekoDL                                ← FETCH: torrent/yt-dlp/Booth/Plex-rip download
                          │
                          │  writes into shared media volume
                          │  (+ optional "scan now" notify call)
                          ▼
                  Tdarr Server / Node                        ← TRANSCODE: FFmpeg/HandBrake flows
                          │
                          ▼
                 Plex / Jellyfin library                     ← WATCH (Ombi shows it as available; NekoDL Channels can build live channels from it)
```

- **Request** — Ombi is the user-facing "request a movie/show" front end. It talks to Radarr/Sonarr/Lidarr directly to create requests and reports back to users once the item shows up on Plex/Jellyfin/Emby. NekoDL doesn't need to integrate with Ombi specifically — it just needs to be a reliable download client at the bottom of the chain Ombi already triggers.
- **Find** — Radarr, Sonarr, and Lidarr already ship with a built-in **Aria2** download-client type. Because NekoDL speaks the same JSON-RPC protocol (see the RPC shim in [TODO.md](TODO.md)), you point each app at NekoDL exactly like you would at aria2 — same host/port, same secret token. Prowlarr never talks to NekoDL directly; it just keeps the indexer lists in Radarr/Sonarr/Lidarr up to date.
- **Fetch** — Categories (`movies`, `tv`, `music`) map to output directories so each app can find and import completed downloads. This path currently targets **torrents only** — Usenet/NZB clients (SABnzbd/NZBGet) are a separate integration NekoDL does not plan to replace.
- **Transcode** — Tdarr is a distributed FFmpeg/HandBrake automation tool with its own Server + Node workers, folder watcher, and plugin-based Flows. NekoDL does no transcoding itself; it shares a media volume with Tdarr and (optionally) pings the Tdarr Server API to trigger an immediate scan on completion, instead of waiting for Tdarr's own schedule.

All these tools stay independent, single-purpose services — NekoDL's job is to be the piece Ombi's requests eventually land on, that Radarr/Sonarr/Lidarr can push to, and that Tdarr can pull from.

## YouTube Subscriptions (Youtarr-inspired)

NekoDL's yt-dlp engine isn't limited to one-off links — it absorbs the subscription-manager feature set popularized by [Youtarr](https://github.com/DialmasterOrg/Youtarr) directly into the core, rather than running Youtarr as a separate service:

- Subscribe to a channel or playlist; new uploads (including Shorts/streams) auto-download on a cron schedule
- SponsorBlock segment removal, per-channel resolution/quality settings, and channel grouping into subfolders for multi-library setups
- NFO files, poster/thumbnail images, and embedded metadata written alongside downloads for Plex/Jellyfin/Emby/Kodi
- Download history with duplicate prevention, plus age/space-based auto-cleanup (with dry-run previews)
- Optional library-refresh call to Plex/Jellyfin/Emby after new videos land, and optional webhook notifications

## Plex Ripping (Pledo-inspired)

A native engine modeled on [Pledo](https://github.com/nekosuneprojects/pledo) for pulling media directly off a Plex server you already have streaming access to — your own, or one a friend has shared with you:

- Log in via plex.tv OAuth — NekoDL never asks for or stores a Plex password
- Sync media metadata from all accessible Plex servers into NekoDL's own local database
- Browse libraries and download movies, TV shows/seasons/episodes, or entire playlists
- Supports multiple file versions per item (different resolutions/codecs) when the server has them
- Background sync keeps track of servers coming online/offline or changing address
- Download server-side (NekoDL fetches from the server directly) or by capturing an in-browser stream, matching Pledo's two modes

## NekoDL Channels (Live TV, Tunarr-inspired)

A NekoDL-native, from-scratch equivalent of [Tunarr](https://tunarr.com/): turn your existing Plex or Jellyfin library into live, scheduled "TV channels."

- **Media sources**: Plex and Jellyfin libraries (Emby parity is a possible future add — see [TODO.md](TODO.md))
- **Channel lineup editor**: drag-and-drop scheduling, filler content between programs, per-channel branding
- **Scheduling**: time-slot and random-slot scheduling, with a web-based guide for viewing lineups
- **Playback/output**: M3U output for IPTV clients, and a spoofed HDHomeRun tuner endpoint so Plex/Jellyfin can auto-discover NekoDL channels as a native "Live TV" source
- **Transcoding**: on-the-fly transcode for playback, with hardware acceleration (NVENC/VAAPI/QuickSync) evaluated for v1 — see [TODO.md](TODO.md) for scope

This is a separate subsystem from the download engines above — it doesn't fetch anything new, it schedules and streams media that's already in your library.

## Design

The dashboard is dark-by-default with a green accent palette, built entirely on Tailwind CSS utility classes and a small set of custom components — no default browser styling, and no native `window.alert()`, `window.confirm()`, or `window.prompt()` calls anywhere in the codebase:

- **Toasts** — custom success/error/warning/info toast components for background notifications (task finished, proxy dropped, import failed).
- **Modals** — custom dialog components for confirmations and input prompts (delete task, edit proxy config, enter a Booth token), fully styled, keyboard-accessible, and focus-trapped.
- **Component kit** — buttons, inputs, dropdowns, progress bars, and badges all themed consistently rather than left as unstyled browser defaults.

This is treated as a hard rule during Web GUI development (Phase 6 in [TODO.md](TODO.md)), not a nice-to-have — every user-facing confirmation or notification goes through NekoDL's own components.

## Quick Start

> Not available yet — this section will be filled in once the core engine and Docker image ship. Planned shape:

```bash
docker compose up -d
# then open http://localhost:6900
```

## Credits

- [aria2](https://github.com/aria2/aria2) — the design this project is inspired by.
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) — the media extraction engine NekoDL wraps.
- [BoothDownloader](https://github.com/Myrkie/BoothDownloader) by [Myrkie](https://github.com/Myrkie) — Booth.pm asset downloader NekoDL integrates with.
- [anacrolix/torrent](https://github.com/anacrolix/torrent) — candidate pure-Go BitTorrent library.
- [gluetun](https://github.com/qdm12/gluetun) — reference VPN sidecar for Docker deployments.
- [Radarr](https://radarr.video/), [Sonarr](https://sonarr.tv/), [Lidarr](https://lidarr.audio/), [Prowlarr](https://prowlarr.com/) — the *arr suite NekoDL integrates with as a download client.
- [Tdarr](https://home.tdarr.io/) — the transcode automation server NekoDL hands finished media off to.
- [Youtarr](https://github.com/DialmasterOrg/Youtarr) by [DialmasterOrg](https://github.com/DialmasterOrg) — inspiration for NekoDL's YouTube channel/playlist subscription features.
- [Tunarr](https://tunarr.com/) — inspiration for the NekoDL Channels live-TV feature.
- [Pledo](https://github.com/nekosuneprojects/pledo) — inspiration for the Plex ripping engine.
- [Ombi](https://ombi.io/) — the request front end NekoDL's *arr-facing download client is designed to sit underneath.

## License

[MIT](LICENSE).

## Contributing

This project is in the planning stage — see [TODO.md](TODO.md) for the current roadmap and [CHANGELOG.md](CHANGELOG.md) for progress. Issues and design discussion are welcome once the repo has its first commits.
