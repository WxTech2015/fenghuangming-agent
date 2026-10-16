#!/system/bin/sh
MODDIR=${0%/scripts/*}
DATA=/data/adb/fenghuangming
case "$1" in start|stop|install)
  "$MODDIR/scripts/manage.sh" "$1"
  ;;
  request)
    [ "${#2}" -le 32768 ] || exit 1
    case "$2" in ''|*[!A-Za-z0-9+/=]*) exit 1;; esac
    "$MODDIR/bin/fhm-agent" --data "$DATA" --request "$2"
    ;;
  *) echo "不支持的管理操作" >&2; exit 1 ;;
esac
