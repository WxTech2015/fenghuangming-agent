//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type nativePlayer struct {
	*net.UnixConn
	mu  sync.Mutex
	uid uint32
}

// 文件由 APK 创建，保留其应用 UID 与 SELinux 标签；Root 只填充已有缓冲文件。
func stageAudio(source string, uid uint32) (*os.File, error) {
	input, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 200*1024*1024 {
		return nil, errors.New("音频缓存无效")
	}
	directory := "/data/user/0/" + packageName + "/cache/fenghuangming"
	if err = checkSpace(directory, info.Size()+256*1024*1024); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(filepath.Join(directory, "incoming.audio"), syscall.O_WRONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errors.New("播放器私有缓冲未就绪")
	}
	output := os.NewFile(uintptr(fd), "player-audio")
	defer output.Close()
	var stat syscall.Stat_t
	if syscall.Fstat(fd, &stat) != nil || stat.Uid != uid || stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Mode&0077 != 0 {
		return nil, errors.New("播放器缓冲身份或权限不正确")
	}
	if err = output.Truncate(0); err != nil {
		return nil, err
	}
	n, err := io.Copy(output, io.LimitReader(input, 200*1024*1024+1))
	if err != nil || n != info.Size() {
		return nil, errors.New("写入播放器缓冲失败")
	}
	if err = output.Sync(); err != nil {
		return nil, err
	}
	return os.Open(fmt.Sprintf("/proc/self/fd/%d", fd))
}

func lockDaemon(data string) (func(), error) {
	if err := os.MkdirAll(data, 0700); err != nil {
		return nil, err
	}
	return lockFile(filepath.Join(data, "daemon.lock"))
}

func lockFile(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("操作已经在运行")
	}
	return func() { _ = file.Close() }, nil
}

func checkSpace(directory string, required int64) error {
	var info syscall.Statfs_t
	if syscall.Statfs(directory, &info) != nil {
		return errors.New("无法检查缓存可用空间")
	}
	available := uint64(info.Bavail) * uint64(info.Bsize)
	if available < uint64(required) {
		return errors.New("缓存空间不足，保留至少 256 MB 可用空间")
	}
	return nil
}

func (p *nativePlayer) Send(value map[string]any, file string) error {
	bytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	p.mu.Lock()
	defer p.mu.Unlock()
	timeout := 3 * time.Second
	if value["action"] == "brake" && value["blocked"] == true {
		timeout = 250 * time.Millisecond
	}
	_ = p.SetWriteDeadline(time.Now().Add(timeout))
	if file != "" {
		f, err := stageAudio(file, p.uid)
		if err != nil {
			return err
		}
		defer f.Close()
		actual, err := hashFile(fmt.Sprintf("/proc/self/fd/%d", f.Fd()))
		if err != nil || actual != value["sha256"] {
			return errors.New("播放器缓冲校验失败")
		}
	}
	_, err = p.Write(bytes)
	return err
}
func peerUID(connection *net.UnixConn) (uint32, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return 0, err
	}
	var cred *syscall.Ucred
	var inner error
	err = raw.Control(func(fd uintptr) {
		cred, inner = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return 0, err
	}
	if inner != nil {
		return 0, inner
	}
	return cred.Uid, nil
}
func installPlayer(data, module string) error {
	unlock, err := lockFile(filepath.Join(data, "player-install.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := installedUID(module); err == nil {
		fmt.Println("播放组件已安装，版本校验通过")
		return nil
	}
	if err := streamPlayerAPK(module, runAndroidCommand); err != nil {
		return err
	}
	if _, err := installedUID(module); err != nil {
		return fmt.Errorf("安装后校验失败：%w", err)
	}
	fmt.Println("播放组件安装成功，版本与应用身份校验通过")
	return nil
}
func installedUID(module string) (int, error) {
	expected, err := hashFile(filepath.Join(module, "assets", "player.apk"))
	if err != nil {
		return 0, errors.New("模块内缺少播放 APK")
	}
	installed, err := commandOutput("/system/bin/pm", "path", "--user", "0", packageName)
	if err != nil {
		return 0, err
	}
	var base string
	for _, line := range strings.Split(installed, "\n") {
		if strings.HasPrefix(line, "package:") && strings.HasSuffix(strings.TrimSpace(line), "/base.apk") {
			base = strings.TrimSpace(strings.TrimPrefix(line, "package:"))
			break
		}
	}
	if !strings.HasPrefix(base, "/data/app/") {
		return 0, errors.New("请先在模块中安装播放组件")
	}
	actual, err := hashFile(base)
	if err != nil || actual != expected {
		return 0, errors.New("播放组件版本或签名不匹配，请重新安装模块组件")
	}
	value, err := commandOutput("/system/bin/cmd", "package", "list", "packages", "-U", "--user", "0", packageName)
	if err != nil {
		return 0, err
	}
	pattern := regexp.MustCompile(`(?m)^package:` + regexp.QuoteMeta(packageName) + ` uid:([0-9]+)\s*$`)
	match := pattern.FindStringSubmatch(value)
	if match == nil {
		return 0, errors.New("无法确认播放应用身份")
	}
	uid, err := strconv.Atoi(match[1])
	if err != nil || uid < 10000 {
		return 0, errors.New("播放应用 UID 无效")
	}
	return uid, nil
}

// 只豁免本项目的播放器；原有白名单不归模块所有，卸载时不删除。
func ensurePlayerKeepalive(data string) error {
	output, err := commandOutput("/system/bin/dumpsys", "deviceidle", "whitelist")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Split(strings.TrimSpace(line), ",")
		if len(parts) >= 2 && parts[1] == packageName {
			return nil
		}
	}
	marker := filepath.Join(data, "keepalive-whitelist.added")
	if err := atomicJSON(marker, map[string]bool{"added": true}); err != nil {
		return err
	}
	_, err = commandOutput("/system/bin/dumpsys", "deviceidle", "whitelist", "+"+packageName)
	return err
}

func releaseKeepalive(data string) error {
	marker := filepath.Join(data, "keepalive-whitelist.added")
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := commandOutput("/system/bin/dumpsys", "deviceidle", "whitelist", "-"+packageName); err != nil {
		return err
	}
	return os.Remove(marker)
}

func connectPlayer(data, module string) (playerConnection, error) {
	uid, err := installedUID(module)
	if err != nil {
		return nil, err
	}
	if err := ensurePlayerKeepalive(data); err != nil {
		return nil, fmt.Errorf("无法设置播放器待机白名单：%w", err)
	}
	name := fmt.Sprintf("@fenghuangming.player.v1.%d", uid)
	connection, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: name, Net: "unix"})
	if err != nil {
		if _, startErr := commandOutput("/system/bin/am", "start-foreground-service", "--user", "0", "-n", packageName+"/.PlaybackService"); startErr != nil {
			return nil, startErr
		}
		return nil, errors.New("原生服务尚未就绪，可在 WebUI 点击启动播放器")
	}
	peer, err := peerUID(connection)
	if err != nil || int(peer) != uid {
		connection.Close()
		return nil, errors.New("拒绝未知本机播放器")
	}
	return &nativePlayer{UnixConn: connection, uid: uint32(uid)}, nil
}
func listenControl(path string) (net.Listener, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("控制路径不是套接字")
		}
		probe, e := net.DialTimeout("unix", path, time.Second)
		if e == nil {
			probe.Close()
			return nil, errors.New("守护进程已经启动")
		}
		if err = os.Remove(path); err != nil {
			return nil, err
		}
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err == nil {
		err = os.Chmod(path, 0600)
	}
	return listener, err
}
func controlPeer(c net.Conn) bool {
	unix, ok := c.(*net.UnixConn)
	if !ok {
		return false
	}
	uid, err := peerUID(unix)
	return err == nil && uid == 0
}
func activatePlayer() error {
	_, err := commandOutput("/system/bin/am", "start", "--user", "0", "-n", packageName+"/.BootstrapActivity")
	return err
}
func stopPlayer() error {
	_, err := commandOutput("/system/bin/am", "force-stop", "--user", "0", packageName)
	return err
}
