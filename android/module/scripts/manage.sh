#!/system/bin/sh
MODDIR=${0%/scripts/*}
DATA=/data/adb/fenghuangming
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
  AGENT_VERSION=$("$BIN" --version)
  echo "$(date +%Y-%m-%dT%H:%M:%S%z) [$AGENT_VERSION] 检查播放组件"
  "$BIN" --data "$DATA" --module "$MODDIR" --install-player
}
stop_agent() {
  if valid_pid; then kill -TERM "$PID"; fi
  # CLI 的 Android 命令使用独立管道，避免把当前日志文件传给 Binder。
  "$BIN" --stop-player >/dev/null 2>&1
}
case "$1" in
  supervise)
    WAIT=0
    while [ "$(getprop sys.boot_completed)" != "1" ] && [ "$WAIT" -lt 60 ]; do
      [ -f "$MODDIR/disable" ] && exit 0
      sleep 2; WAIT=$((WAIT+1))
    done
    [ "$(getprop sys.boot_completed)" = "1" ] || { echo "系统尚未就绪，停止自动启动"; exit 1; }
    install_player || echo "播放组件安装失败；守护仍会启动，可在 WebUI 查看日志并重试安装"
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
  install)
    rotate_log "$DATA/launcher.log"
    install_player >> "$DATA/launcher.log" 2>&1 </dev/null &
    echo "已提交播放组件安装，请在日志查看结果"
    ;;
  *) echo "不支持的服务操作" >&2; exit 1 ;;
esac
