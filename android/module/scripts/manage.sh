#!/system/bin/sh
MODDIR=${0%/scripts/*}
DATA=/data/adb/fenghuangming
BIN="$MODDIR/bin/fhm-agent"
BUSYBOX=/data/adb/ksu/bin/busybox
launch_supervisor() {
  rotate_log "$DATA/launcher.log"
  "$BUSYBOX" nohup "$BUSYBOX" setsid "$0" supervise </dev/null >> "$DATA/launcher.log" 2>&1 &
}
detach_manager() {
  # 只迁移当前监控进程；后续子进程继承独立 cgroup，不跟随管理器退出。
  for GROUP in /acct /dev/cg2_bpf /sys/fs/cgroup /dev/memcg/apps; do
    [ -f "$GROUP/cgroup.procs" ] || continue
    if ! (printf '%s\n' "$$" > "$GROUP/cgroup.procs") 2>/dev/null; then
      echo "无法脱离管理器 cgroup：$GROUP"
    fi
  done
  echo "监控进程 cgroup："
  cat /proc/$$/cgroup
  (printf '%s\n' -800 > /proc/$$/oom_score_adj) 2>/dev/null || echo "无法设置监控进程优先级"
  trap '' HUP
}
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
    detach_manager
    # KernelSU 可能重复触发开机钩子；只保留一个监控进程。
    exec "$BUSYBOX" flock -n "$DATA/supervisor.lock" "$0" supervise-locked
    ;;
  supervise-locked)
    while [ "$(getprop sys.boot_completed)" != "1" ]; do
      if [ -f "$MODDIR/disable" ] || [ -f "$MODDIR/remove" ] || [ -f "$DATA/service.stopped" ]; then exit 0; fi
      sleep 2
    done
    install_player || echo "播放组件安装失败；守护仍会启动，可在 WebUI 查看日志并重试安装"
    FAILURES=0
    while [ ! -f "$MODDIR/disable" ] && [ ! -f "$MODDIR/remove" ] && [ ! -f "$DATA/service.stopped" ]; do
      if valid_pid; then sleep 5; continue; fi
      STARTED=$(date +%s)
      rotate_log "$DATA/daemon.log"
      "$BIN" --data "$DATA" --module "$MODDIR" >> "$DATA/daemon.log" 2>&1 &
      PID=$!
      printf '%s\n' "$PID" > "$DATA/daemon.pid"
      chmod 600 "$DATA/daemon.pid"
      wait "$PID"
      RESULT=$?
      ELAPSED=$(( $(date +%s)-STARTED ))
      if [ -f "$MODDIR/disable" ] || [ -f "$MODDIR/remove" ] || [ -f "$DATA/service.stopped" ]; then break; fi
      [ "$ELAPSED" -ge 600 ] && FAILURES=0
      [ "$FAILURES" -ge 15 ] || FAILURES=$((FAILURES+1))
      echo "$(date +%Y-%m-%dT%H:%M:%S%z) 守护已退出：code=$RESULT，运行 ${ELAPSED}s；未停用时持续重试"
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
    launch_supervisor
    ;;
  launch)
    launch_supervisor
    ;;
  install)
    rotate_log "$DATA/launcher.log"
    install_player >> "$DATA/launcher.log" 2>&1 </dev/null &
    echo "已提交播放组件安装，请在日志查看结果"
    ;;
  *) echo "不支持的服务操作" >&2; exit 1 ;;
esac
