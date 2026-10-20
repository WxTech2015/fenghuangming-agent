#!/system/bin/sh
[ "$KSU" = "true" ] || abort "请通过 SukiSU / KernelSU 管理器安装"
MIN_API=29
MIN_ANDROID=10
[ "$API" -ge "$MIN_API" ] || abort "需要 Android $MIN_ANDROID 或更新版本"
[ "$ARCH" = "arm64" ] || abort "当前版本仅支持 ARM64"
DATA=/data/adb/fenghuangming
[ ! -L "$DATA" ] || abort "数据目录不能是符号链接"
mkdir -p "$DATA"
chmod 700 "$DATA"
if [ ! -f "$DATA/control.json" ]; then
  printf '%s\n' '{"blocked":true}' > "$DATA/control.json"
  chmod 600 "$DATA/control.json"
fi
set_perm "$MODPATH/bin/fhm-agent" 0 0 0755
set_perm "$MODPATH/scripts/manage.sh" 0 0 0755
set_perm "$MODPATH/scripts/ctl.sh" 0 0 0755
set_perm "$MODPATH/service.sh" 0 0 0755
set_perm "$MODPATH/uninstall.sh" 0 0 0755
touch "$MODPATH/skip_mount"
ui_print "安装完成。重启后打开模块 WebUI，配置端点和设备凭证，再手动解除制动。"
ui_print "当前安装不会改写引导、分区或系统应用。"
