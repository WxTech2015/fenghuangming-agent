#!/system/bin/sh
[ "$(id -u)" = "0" ] || { echo "请使用 Root 执行" >&2; exit 1; }
DATA=/data/adb/fenghuangming
APP="$DATA/standalone"
BIN="$APP/bin/fhm-agent"
BUSYBOX=/data/adb/ksu/bin/busybox
SELF="$APP/fhm.sh"
[ -x "$BIN" ] && [ -x "$BUSYBOX" ] || { echo "程序或 SukiSU BusyBox 不完整，请重新安装" >&2; exit 1; }
request() { "$BIN" --data "$DATA" --request "$(printf '%s' "$1" | "$BUSYBOX" base64 | tr -d '\n')"; }
valid_console() {
  [ -f "$DATA/console.pid" ] || return 1
  CONSOLE_PID=$(cat "$DATA/console.pid")
  case "$CONSOLE_PID" in ''|*[!0-9]*) return 1;; esac
  [ "$CONSOLE_PID" -gt 1 ] || return 1
  EXE=$(readlink "/proc/$CONSOLE_PID/exe")
  [ "$EXE" = "$BIN" ] || [ "$EXE" = "$BIN (deleted)" ]
}
console_start() {
  "$BIN" --data "$DATA" --console-url >/dev/null || return 1
  rm -f "$DATA/console.stopped"
  valid_console && return 0
  "$BUSYBOX" nohup "$BUSYBOX" setsid /system/bin/sh "$SELF" console-supervise </dev/null >> "$DATA/console.log" 2>&1 &
}
case "$1" in
  start)
    /system/bin/sh "$APP/scripts/manage.sh" start || exit 1
    console_start
    echo "已启动。运行 sh $SELF open 打开本机控制台。"
    ;;
  launch)
    /system/bin/sh "$APP/scripts/manage.sh" launch
    [ -f "$DATA/console.stopped" ] || console_start
    ;;
  console-supervise)
    for GROUP in /acct /dev/cg2_bpf /sys/fs/cgroup /dev/memcg/apps; do
      [ ! -f "$GROUP/cgroup.procs" ] || (printf '%s\n' "$$" > "$GROUP/cgroup.procs") 2>/dev/null
    done
    (printf '%s\n' -800 > /proc/$$/oom_score_adj) 2>/dev/null
    trap '' HUP
    exec "$BUSYBOX" flock -n "$DATA/console-supervisor.lock" /system/bin/sh "$SELF" console-loop
    ;;
  console-loop)
    while [ ! -f "$DATA/console.stopped" ]; do
      if [ -f "$DATA/console.log" ] && [ "$(wc -c < "$DATA/console.log")" -ge 1048576 ]; then mv -f "$DATA/console.log" "$DATA/console.log.1"; fi
      "$BIN" --data "$DATA" --module "$APP" --console >> "$DATA/console.log" 2>&1 &
      printf '%s\n' "$!" > "$DATA/console.pid"
      chmod 600 "$DATA/console.pid"
      wait "$!"
      [ -f "$DATA/console.stopped" ] || sleep 5
    done
    ;;
  stop)
    /system/bin/sh "$APP/scripts/manage.sh" stop
    echo "播放服务已停止，控制台仍可打开。"
    ;;
  shutdown)
    /system/bin/sh "$APP/scripts/manage.sh" stop
    touch "$DATA/console.stopped"
    if valid_console; then kill -TERM "$CONSOLE_PID"; fi
    ;;
  status) request '{"action":"state"}' ;;
  brake) request '{"action":"brake"}' ;;
  release) request '{"action":"release"}' ;;
  logs) request '{"action":"logs"}' ;;
  open|url)
    console_start
    URL=$("$BIN" --data "$DATA" --console-url) || exit 1
    if [ "$1" = "url" ]; then printf '%s\n' "$URL"; else
      /system/bin/am start --user 0 -a android.intent.action.VIEW -d "$URL" >/dev/null 2>&1 || { echo "无法自动打开浏览器，运行 sh $SELF url 获取登录地址" >&2; exit 1; }
    fi
    ;;
  uninstall)
    /system/bin/sh "$SELF" shutdown
    "$BIN" --data "$DATA" --release-keepalive
    HOOK=/data/adb/service.d/fenghuangming-standalone.sh
    if [ -f "$HOOK" ] && grep -q '^# fenghuangming standalone boot hook$' "$HOOK"; then rm -f "$HOOK"; fi
    echo "已停止并移除本项目开机脚本。保留配置、程序文件和播放 APK。"
    ;;
  *) echo "用法：sh $SELF start|stop|status|open|url|brake|release|logs|shutdown|uninstall"; exit 1 ;;
esac
