#!/system/bin/sh
MODDIR=${0%/*}
"$MODDIR/scripts/manage.sh" stop
# 保留配置、制动记录、日志与播放 APK，避免卸载误删用户数据。
