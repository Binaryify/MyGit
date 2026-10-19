#!/bin/sh
# add-readme.sh <dmg>: puts the install instructions under the app in a disk
# image made by mygo build, which holds only the app and Applications. The
# window grows to show them, and the icons rise to make room. Needs the
# ds-store Python package (pip install ds-store).
set -eu
dmg=$1
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'hdiutil detach -quiet "$work/mnt" 2>/dev/null || true; rm -rf "$work"' EXIT

# A compressed image is read-only: convert it, with room for the file.
hdiutil convert "$dmg" -format UDRW -o "$work/rw.dmg" -quiet
hdiutil resize -size "$(( $(stat -f %z "$work/rw.dmg") / 1048576 + 2 ))m" "$work/rw.dmg" 2>/dev/null
mkdir "$work/mnt"
hdiutil attach "$work/rw.dmg" -mountpoint "$work/mnt" -nobrowse -noautoopen -quiet
cp "$here/"*.txt "$work/mnt/"
python3 - "$work/mnt/.DS_Store" "$(basename "$here/"*.txt)" <<'PY'
import sys
from ds_store import DSStore
path, readme = sys.argv[1], sys.argv[2]
with DSStore.open(path, "r+") as d:
    bwsp = d["."]["bwsp"]
    bwsp["WindowBounds"] = "{{200, 340}, {660, 540}}"
    d["."]["bwsp"] = bwsp
    d["MyGit.app"]["Iloc"] = (180, 150)
    d["Applications"]["Iloc"] = (480, 145)
    d[readme]["Iloc"] = (330, 340)
PY
hdiutil detach -quiet "$work/mnt"
hdiutil convert "$work/rw.dmg" -format ULMO -ov -o "$dmg" -quiet
