#!/bin/sh
# Fixes /data ownership before dropping to the non-root nekodl user, then
# execs the real command. This runs on every container start, not just
# once at image build time, because a mounted volume's ownership isn't
# controlled by the image: a bind mount takes whatever the host directory
# already has (often root, or whatever created it first), and even a named
# volume that was first populated by an older image (or one that ran as
# root) keeps that ownership across upgrades — Docker only auto-copies the
# image's own ownership into a volume the *first* time it's used, not on
# every subsequent run. Fixing it here handles both cases instead of
# assuming a fresh volume every time.
set -e

mkdir -p /data/downloads /data/torrents
chown -R nekodl:nekodl /data

exec su-exec nekodl:nekodl "$@"
