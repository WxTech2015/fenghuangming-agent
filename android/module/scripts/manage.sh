#!/system/bin/sh
MODDIR=${0%/scripts/*}
DATA=/data/adb/fenghuangming
PKG=com.phoring.fenghuangming.player
BIN="$MODDIR/bin/fhm-agent"
rotate_log() {
  [ -f "$1" ] && [ ! -L "$1" ] || return 0
  SIZE=$(wc -c < "$1")
  case "$SIZE" in ''|*[!0-9]*) return 0;; esac
  [ "$SIZE" -lt 1048576 ] || mv -f "$1" "$1.1"
}
valid_pid() {
  [ -f "$DATA/daemon.pid" ] || return 1
  PID=$(cat "$DATA/daemon.pid")
  case "$PID" in ''|*[!0-9]*) return 1;; esac
  [ "$PID" -gt 1 ] || return 1
  EXE=$(readlink "/proc/$PID/exe")
  [ "$EXE" = "$BIN" ] || [ "$EXE" = "$BIN (deleted)" ]
}
install_player() {
  [ "$(getprop sys.boot_completed)" = "1" ] || return 1
  EXPECTED=$(cat "$MODDIR/assets/player.sha256")
  ACTUAL=$(sha256sum "$MODDIR/assets/player.apk")
  ACTUAL=${ACTUAL%% *}
  [ "$ACTUAL" = "$EXPECTED" ] || { echo "播放 APK 校验失败"; return 1; }
  INSTALLED=$(/system/bin/pm path --user 0 "$PKG")
  case "$INSTALLED" in package:*/base.apk)
    CURRENT=${INSTALLED#package:}
    CURRENT_HASH=$(sha256sum "$CURRENT")
    [ "${CURRENT_HASH%% *}" = "$EXPECTED" ] && return 0
    ;;
  esac
  /system/bin/pm install -r --user 0 "$MODDIR/assets/player.apk"
}
stop_agent() {
  if valid_pid; then kill -TERM "$PID"; fi
  /system/bin/am force-stop --user 0 "$PKG" >/dev/null 2>&1
}
case "$1" in
  supervise)
    WAIT=0
    while [ "$(getprop sys.boot_completed)" != "1" ] && [ "$WAIT" -lt 60 ]; do
      [ -f "$MODDIR/disable" ] && exit 0
      sleep 2; WAIT=$((WAIT+1))
    done
    [ "$(getprop sys.boot_completed)" = "1" ] || { echo "系统尚未就绪，停止自动启动"; exit 1; }
    install_player || { echo "播放组件安装失败，请在 WebUI 查看日志"; exit 1; }
    FAILURES=0
    while [ ! -f "$MODDIR/disable" ] && [ ! -f "$MODDIR/remove" ] && [ ! -f "$DATA/service.stopped" ]; do
      if valid_pid; then sleep 5; continue; fi
      [ "$FAILURES" -lt 5 ] || { echo "守护程序连续退出，已停止自动恢复"; exit 1; }
      STARTED=$(date +%s)
      rotate_log "$DATA/daemon.log"
      "$BIN" --data "$DATA" --module "$MODDIR" >> "$DATA/daemon.log" 2>&1 &
      PID=$!
      printf '%s\n' "$PID" > "$DATA/daemon.pid"
      chmod 600 "$DATA/daemon.pid"
      wait "$PID"
      ELAPSED=$(( $(date +%s)-STARTED ))
      [ "$ELAPSED" -ge 600 ] && FAILURES=0
      FAILURES=$((FAILURES+1))
      sleep $((FAILURES*2))
    done
    stop_agent
    ;;
  stop)
    touch "$DATA/service.stopped"
    stop_agent
    ;;
  start)
    rm -f "$DATA/service.stopped"
    if valid_pid; then exit 0; fi
    rotate_log "$DATA/launcher.log"
    "$0" supervise >> "$DATA/launcher.log" 2>&1 &
    ;;
  install) install_player ;;
  *) echo "不支持的服务操作" >&2; exit 1 ;;
esac
