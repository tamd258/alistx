package wxyunpan

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/http_range"
)

const (
	// wxRetryTimes 单次 Range 读取最多重连次数
	wxRetryTimes = 20
	// wxRetryBaseMS 重连退避基数（毫秒），按次数线性递增，封顶 4s
	wxRetryBaseMS = 400
)

// wxResumableLink 给 Link 装一个「断了就按 Range 续」的 RangeReadCloser。
//
// 背景：PikPak 的下载直链会随机在任意偏移处被服务端掐断
//   - 单连接整条读：实测 ~69MB / ~79MB 处被关，net/http 抛 unexpected EOF
//   - 8MB 分片读：约 1/4 的请求也会被截断（IncompleteRead）
//   - 还有一种「假活」：TCP 连着但一个字节都不给，不设读超时就永远阻塞
//
// 而 139 云盘（personal_new）上传前必须算整文件 SHA256，走 stream.CacheFullInTempFile
// 一次性顺序读完，任何一次中断都会让它整体失败，报
// "unexpected EOF; CreateTempFile failed"。
//
// 所以这里把读取做成可续传：每次底层断流就从当前偏移重新发 Range 请求接着读，
// 对上层是一个连续的 io.ReadCloser。
func wxResumableLink(link *model.Link, size int64) *model.RangeReadCloser {
	return &model.RangeReadCloser{
		RangeReader: func(ctx context.Context, r http_range.Range) (io.ReadCloser, error) {
			length := r.Length
			if length < 0 && size > 0 {
				length = size - r.Start
			}
			if length < 0 {
				length = -1
			}
			return &wxResumeReader{
				ctx:    ctx,
				url:    link.URL,
				header: link.Header,
				start:  r.Start,
				length: length,
			}, nil
		},
	}
}

type wxResumeReader struct {
	ctx    context.Context
	url    string
	header http.Header
	start  int64
	length int64 // -1 表示直到文件末尾
	off    int64 // 相对 start 已交付的字节
	body   io.ReadCloser
	closed bool
}

func (r *wxResumeReader) open() error {
	if r.closed {
		return io.EOF
	}
	begin := r.start + r.off
	if r.length >= 0 && r.off >= r.length {
		return io.EOF
	}
	// 注意：一定要发「开放区间」bytes=N-。
	// 实测 PikPak 对「闭合区间」bytes=N-M 会提前掐断并限速（整文件闭合区间 0.77MB/s，
	// 同一文件开放区间 4.9MB/s），剩下的长度由 Read 自己数着截断。
	spec := fmt.Sprintf("bytes=%d-", begin)
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return err
	}
	for k, vs := range r.header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ua)
	}
	req.Header.Set("Range", spec)
	resp, err := wxHttpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_ = resp.Body.Close()
		return fmt.Errorf("wxyunpan: http %d", resp.StatusCode)
	}
	r.body = resp.Body
	return nil
}

func (r *wxResumeReader) Read(p []byte) (int, error) {
	if r.length >= 0 {
		remain := r.length - r.off
		if remain <= 0 {
			return 0, io.EOF
		}
		// 开放区间读多了不要——按剩下的长度截断，避免越过本次 Range 的范围
		if int64(len(p)) > remain {
			p = p[:remain]
		}
	}
	var lastErr error
	for attempt := 0; attempt <= wxRetryTimes; attempt++ {
		if r.body == nil {
			if err := r.open(); err != nil {
				if err == io.EOF && (r.length < 0 || r.off >= r.length) {
					return 0, io.EOF
				}
				lastErr = err
				time.Sleep(r.backoff(attempt))
				continue
			}
		}
		n, err := r.body.Read(p)
		r.off += int64(n)
		if n > 0 {
			// 先把读到的数据交出去，错误留到下一次 Read 处理
			return n, nil
		}
		if err == nil {
			continue
		}
		// 断流 / 提前 EOF / 连接被重置：关掉旧连接，下一轮从当前位置续传
		_ = r.body.Close()
		r.body = nil
		lastErr = err
		if r.length >= 0 && r.off >= r.length {
			return 0, io.EOF
		}
		if r.length < 0 && err == io.EOF {
			return 0, io.EOF
		}
		time.Sleep(r.backoff(attempt))
	}
	if lastErr == nil {
		lastErr = io.ErrUnexpectedEOF
	}
	return 0, fmt.Errorf("wxyunpan: 续传 %d 次仍失败（已读 %d 字节）: %w", wxRetryTimes, r.off, lastErr)
}

func (r *wxResumeReader) backoff(attempt int) time.Duration {
	d := time.Duration(attempt+1) * wxRetryBaseMS * time.Millisecond
	if d > 4*time.Second {
		d = 4 * time.Second
	}
	return d
}

func (r *wxResumeReader) Close() error {
	r.closed = true
	if r.body != nil {
		err := r.body.Close()
		r.body = nil
		return err
	}
	return nil
}
