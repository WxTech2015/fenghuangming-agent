#!/system/bin/sh
set -eu
[ "$(id -u)" = "0" ] || { echo "请使用 Root 执行安装脚本" >&2; exit 1; }
DATA=/data/adb/fenghuangming
APP="$DATA/standalone"
BUSYBOX=/data/adb/ksu/bin/busybox
[ -x "$BUSYBOX" ] || { echo "未找到 SukiSU / KernelSU BusyBox" >&2; exit 1; }
API=$(getprop ro.build.version.sdk)
[ "$API" -ge 28 ] || { echo "需要 Android 9 或更新版本" >&2; exit 1; }
[ "$(getprop ro.product.cpu.abi)" = "arm64-v8a" ] || { echo "此安装包仅支持 ARM64" >&2; exit 1; }
[ ! -L "$DATA" ] && [ ! -L "$APP" ] && [ ! -L /data/adb/service.d ] || { echo "安装目录不能是符号链接" >&2; exit 1; }
umask 077
mkdir -p "$DATA"
STAGE=$("$BUSYBOX" mktemp -d "$DATA/standalone-install.XXXXXX")
cleanup() { case "$STAGE" in "$DATA"/standalone-install.*) rm -rf "$STAGE";; esac; }
trap cleanup EXIT
echo "正在解包并校验程序……"
awk 'payload { print } /^__FHM_PAYLOAD_BELOW__$/ { payload=1 }' "$0" | "$BUSYBOX" base64 -d > "$STAGE/payload.zip"
ACTUAL=$("$BUSYBOX" sha256sum "$STAGE/payload.zip" | cut -d ' ' -f 1)
[ "$ACTUAL" = "__PAYLOAD_SHA256__" ] || { echo "安装包校验失败，请重新下载" >&2; exit 1; }
mkdir "$STAGE/app"
"$BUSYBOX" unzip -oq "$STAGE/payload.zip" -d "$STAGE/app"
for FILE in fhm.sh scripts/manage.sh scripts/ctl.sh bin/fhm-agent assets/player.apk webroot/index.html; do
  [ -f "$STAGE/app/$FILE" ] || { echo "安装包缺少 $FILE" >&2; exit 1; }
done
chmod 700 "$STAGE/app/bin/fhm-agent" "$STAGE/app/fhm.sh" "$STAGE/app/scripts/"*.sh
"$STAGE/app/bin/fhm-agent" --version
if [ -x "$APP/bin/fhm-agent" ] && [ -f "$APP/fhm.sh" ]; then /system/bin/sh "$APP/fhm.sh" shutdown; fi
# 只停用本项目旧模块，避免模块版和独立版同时运行。
for OLD in /data/adb/modules/fenghuangming_agent /data/adb/modules_update/fenghuangming_agent; do
  if [ -d "$OLD" ] && [ ! -L "$OLD" ]; then
    touch "$OLD/disable"
    if [ -x "$OLD/bin/fhm-agent" ] && [ -f "$OLD/scripts/manage.sh" ]; then /system/bin/sh "$OLD/scripts/manage.sh" stop; fi
  fi
done
if [ -f "$DATA/daemon.pid" ]; then
  PID=$(cat "$DATA/daemon.pid")
  case "$PID" in ''|*[!0-9]*) ;; *)
    for WAIT in 1 2 3 4 5 6 7 8 9 10; do
      EXE=$(readlink "/proc/$PID/exe" 2>/dev/null || true)
      case "$EXE" in "$APP/bin/fhm-agent"|"$APP/bin/fhm-agent (deleted)"|/data/adb/modules/fenghuangming_agent/bin/fhm-agent*) sleep 1;; *) break;; esac
    done
    EXE=$(readlink "/proc/$PID/exe" 2>/dev/null || true)
    case "$EXE" in "$APP/bin/fhm-agent"|"$APP/bin/fhm-agent (deleted)"|/data/adb/modules/fenghuangming_agent/bin/fhm-agent*) echo "旧守护尚未停止，请稍后重试" >&2; exit 1;; esac
  ;; esac
fi
BACKUP="$DATA/standalone.previous"
[ ! -e "$BACKUP" ] || { echo "发现上次安装备份，请先检查 $BACKUP" >&2; exit 1; }
if [ -d "$APP" ]; then mv "$APP" "$BACKUP"; fi
if ! mv "$STAGE/app" "$APP"; then
  [ ! -d "$BACKUP" ] || mv "$BACKUP" "$APP"
  echo "程序目录更新失败，已尝试恢复旧版本" >&2; exit 1
fi
# 固定目录内的上一版程序已被替换，配置与缓存不在此目录中。
if [ -d "$BACKUP" ] && [ ! -L "$BACKUP" ]; then rm -rf "$BACKUP"; fi
[ -f "$DATA/control.json" ] || printf '%s\n' '{"blocked":true}' > "$DATA/control.json"
chmod 700 "$DATA" "$APP"
mkdir -p /data/adb/service.d
HOOK=/data/adb/service.d/fenghuangming-standalone.sh
[ ! -L "$HOOK" ] || { echo "开机入口不能是符号链接" >&2; exit 1; }
if [ -f "$HOOK" ] && ! grep -q '^# fenghuangming standalone boot hook$' "$HOOK"; then echo "开机入口已被其他程序使用" >&2; exit 1; fi
cat > "$HOOK" <<'BOOT'
#!/system/bin/sh
# fenghuangming standalone boot hook
/system/bin/sh /data/adb/fenghuangming/standalone/fhm.sh launch
BOOT
chmod 700 "$HOOK"
/system/bin/sh "$APP/fhm.sh" start
echo "安装完成，无需重启。运行以下命令打开控制台："
echo "sh $APP/fhm.sh open"
echo "文件只写入 /data；没有修改 /system、引导或分区。"
exit 0
__FHM_PAYLOAD_BELOW__
