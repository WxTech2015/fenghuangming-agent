package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Transfer struct {
	Bytes       int64 `json:"bytes"`
	Total       int64 `json:"total"`
	Speed       int64 `json:"speedBytesPerSecond"`
	Connections int   `json:"connections"`
}

func hashFile(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
func downloadAudio(ctx context.Context, directory, url, token string, p Payload, connections int, progress func(Transfer)) (string, error) {
	if !hashPattern.MatchString(p.SHA256) || p.Bytes < 1 || p.Bytes > 200*1024*1024 {
		return "", errors.New("音频大小或哈希无效")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	final := filepath.Join(directory, p.SHA256+".audio")
	if info, err := os.Stat(final); err == nil && info.Size() == p.Bytes {
		if digest, err := hashFile(final); err == nil && digest == p.SHA256 {
			return final, nil
		}
	}
	tmp := final + "." + newID() + ".tmp"
	if err := checkSpace(directory, p.Bytes+256*1024*1024); err != nil {
		return "", err
	}
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	defer os.Remove(tmp)
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 180 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("拒绝媒体重定向") }}
	var received atomic.Int64
	started := time.Now()
	var progressMu sync.Mutex
	report := func(n int) {
		progressMu.Lock()
		defer progressMu.Unlock()
		progress(Transfer{received.Load(), p.Bytes, int64(float64(received.Load()) / max(0.01, time.Since(started).Seconds())), n})
	}
	request := func(requestCtx context.Context, start, end int64, ranged bool, n int) error {
		req, e := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
		if e != nil {
			return e
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept-Encoding", "identity")
		if ranged {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
		}
		res, e := client.Do(req)
		if e != nil {
			return errors.New("后端音频连接失败")
		}
		defer res.Body.Close()
		if res.Header.Get("Content-Encoding") != "" {
			return errors.New("音频响应编码无效")
		}
		if ranged {
			if res.StatusCode != 206 || res.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", start, end, p.Bytes) {
				return errors.New("分段响应范围错误")
			}
		} else if res.StatusCode != 200 {
			return fmt.Errorf("音频 HTTP %d", res.StatusCode)
		}
		expected := end - start + 1
		offset := int64(0)
		buf := make([]byte, 64*1024)
		for {
			nread, e := res.Body.Read(buf)
			if nread > 0 {
				if offset+int64(nread) > expected {
					return errors.New("音频超过预期大小")
				}
				if _, e2 := file.WriteAt(buf[:nread], start+offset); e2 != nil {
					return e2
				}
				offset += int64(nread)
				received.Add(int64(nread))
				report(n)
			}
			if e == io.EOF {
				break
			}
			if e != nil {
				return errors.New("音频下载中断")
			}
		}
		if offset != expected {
			return errors.New("音频下载不完整")
		}
		return nil
	}
	connections = max(1, min(4, connections))
	if p.Bytes < 2*1024*1024 {
		connections = 1
	}
	if connections > 1 {
		rangeCtx, stop := context.WithCancel(ctx)
		var group sync.WaitGroup
		errCh := make(chan error, connections)
		chunk := (p.Bytes + int64(connections) - 1) / int64(connections)
		for i := 0; i < connections; i++ {
			group.Add(1)
			go func(index int) {
				defer group.Done()
				start := int64(index) * chunk
				end := min(p.Bytes-1, start+chunk-1)
				if e := request(rangeCtx, start, end, true, connections); e != nil {
					errCh <- e
					stop()
				}
			}(i)
		}
		group.Wait()
		stop()
		close(errCh)
		if len(errCh) > 0 {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if err = file.Truncate(0); err != nil {
				return "", err
			}
			received.Store(0)
			connections = 1
		}
	}
	if connections == 1 {
		if err = request(ctx, 0, p.Bytes-1, false, 1); err != nil {
			return "", err
		}
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = file.Sync(); err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	digest, err := hashFile(tmp)
	if err != nil {
		return "", err
	}
	if digest != p.SHA256 {
		return "", errors.New("音频 SHA-256 不匹配")
	}
	if err = os.Rename(tmp, final); err != nil {
		return "", err
	}
	return final, nil
}
