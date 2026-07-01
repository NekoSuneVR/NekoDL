# NekoDL yt-dlp patches

Drop `*.patch` files (standard `git diff` / `git format-patch` output, applied
with `git apply`) in this directory to carry local fixes for upstream yt-dlp
bugs or site breakage that hasn't been merged/released yet.

There are no patches here today — NekoDL runs stock yt-dlp, pinned to
`YTDLP_VERSION` in the root `Dockerfile`. This directory exists so patching
is a drop-a-file operation instead of a Dockerfile rewrite when one is
eventually needed.

## How it's wired

The `ytdlp-builder` stage in the root `Dockerfile`:

1. `git clone`s yt-dlp at the tag matching `YTDLP_VERSION`.
2. Copies this directory in and applies every `*.patch` file found, in
   filename order (`git apply`) — if none exist, this is a no-op.
3. Builds a wheel from the (possibly patched) source and installs that
   wheel into the runtime image, instead of installing the prebuilt PyPI
   package.

So adding a patch is just: add the file here, bump nothing else, rebuild.

## Why local patch files, not a fork

Upstream yt-dlp ships extractor fixes almost daily (sites change their
player/API internals constantly). A long-lived fork falls behind fast and
turns every `YTDLP_VERSION` bump into a manual rebase. Local patch files
pinned to a specific upstream tag are cheap to drop and cheap to delete once
upstream catches up — check whether a patch is still needed *before*
bumping `YTDLP_VERSION`, since it may already be fixed upstream.

If patches ever accumulate to the point of being unmanageable as loose
files (many, interdependent, or needing their own review history), revisit
this and switch to a real fork with cherry-picked commits instead. That's a
deliberate escalation, not the default.

## Naming and ordering

Patches apply in filename sort order — prefix with a number if order
matters (`01-fix-foo.patch`, `02-fix-bar.patch`).
