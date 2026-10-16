#!/system/bin/sh
MODDIR=${0%/*}
DATA=/data/adb/fenghuangming
mkdir -p "$DATA"
chmod 700 "$DATA"
if [ -f "$DATA/launcher.log" ] && [ ! -L "$DATA/launcher.log" ]; then
  SIZE=$(wc -c < "$DATA/launcher.log")
  case "$SIZE" in ''|*[!0-9]*) ;; *) [ "$SIZE" -lt 1048576 ] || mv -f "$DATA/launcher.log" "$DATA/launcher.log.1";; esac
fi
"$MODDIR/scripts/manage.sh" supervise >> "$DATA/launcher.log" 2>&1 &
