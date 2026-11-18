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
	// 鍗曟 Range 璇诲彇鏈€澶氶噸杩炴鏁?	wxRetryTimes = 20
	// 閲嶈繛閫€閬垮熀鏁帮紙姣锛夛紝鎸夋鏁扮嚎鎬ч€掑锛屽皝椤?4s
	wxRetryBaseMS = 400
)

// wxResumableLink 缁?Link 瑁呬竴涓€屾柇浜嗗氨鎸?Range 缁€嶇殑 RangeReadCloser銆?//
// 鑳屾櫙锛歅ikPak 鐨勪笅杞界洿閾句細闅忔満鍦ㄤ换鎰忓亸绉诲琚湇鍔＄鎺愭柇
//   - 鍗曡繛鎺ユ暣鏉¤锛氬疄娴?~69MB / ~79MB 澶勮鍏筹紝net/http 鎶?unexpected EOF
//   - 8MB 鍒嗙墖璇伙細绾?1/4 鐨勮姹備篃浼氳鎴柇锛圛ncompleteRead锛?//
// 鑰?139 浜戠洏锛坧ersonal_new锛変笂浼犲墠蹇呴』绠楁暣鏂囦欢 SHA256锛岃蛋 stream.CacheFullInTempFile
// 涓€娆℃€ч『搴忚瀹岋紝浠讳綍涓€娆′腑鏂兘浼氳瀹冩暣浣撳け璐ワ紝鎶?// "unexpected EOF; CreateTempFile failed"銆?//
// 鎵€浠ヨ繖閲屾妸璇诲彇鍋氭垚鍙画浼狅細姣忔搴曞眰鏂祦灏变粠褰撳墠鍋忕Щ閲嶆柊鍙?Range 璇锋眰鎺ョ潃璇伙紝
// 瀵逛笂灞傛槸涓€涓繛缁殑 io.ReadCloser銆?func wxResumableLink(link *model.Link, size int64) *model.RangeReadCloser {
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
	length int64 // -1 琛ㄧず鐩村埌鏂囦欢鏈熬
	off    int64 // 鐩稿 start 宸蹭氦浠樼殑瀛楄妭
	body   io.ReadCloser
	closed bool
}

func (r *wxResumeReader) open() error {
	if r.closed {
		return io.EOF
	}
	begin := r.start + r.off
	var spec string
	if r.length >= 0 {
		last := r.start + r.length - 1
		if begin > last {
			return io.EOF
		}
		spec = fmt.Sprintf("bytes=%d-%d", begin, last)
	} else {
		spec = fmt.Sprintf("bytes=%d-", begin)
	}
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
		req.Header.Set("User-Agent", "okhttp/3.12.13")
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
	if r.length >= 0 && r.off >= r.length {
		return 0, io.EOF
	}
	var lastErr error
	for attempt := 0; attempt <= wxRetryTimes; attempt++ {
		if r.body == nil {
			if err := r.open(); err != nil {
				if err == io.EOF {
					if r.length < 0 || r.off >= r.length {
						return 0, io.EOF
					}
				}
				lastErr = err
				time.Sleep(r.backoff(attempt))
				continue
			}
		}
		n, err := r.body.Read(p)
		r.off += int64(n)
		if n > 0 {
			// 鍏堟妸璇诲埌鐨勬暟鎹氦鍑哄幓锛岄敊璇暀鍒颁笅涓€娆?Read 澶勭悊
			return n, nil
		}
		if err == nil {
			continue
		}
		// 鏂祦 / 鎻愬墠 EOF / 杩炴帴琚噸缃細鍏虫帀鏃ц繛鎺ワ紝涓嬩竴杞画浼?		_ = r.body.Close()
		r.body = nil
		lastErr = err
		if r.length >= 0 && r.off >= r.length {
			return 0, io.EOF
		}
		if r.length < 0 && err == io.EOF {
			// 鏈煡鎬婚暱鏃剁殑姝ｅ父缁撴潫
			return 0, io.EOF
		}
		time.Sleep(r.backoff(attempt))
	}
	if lastErr == nil {
		lastErr = io.ErrUnexpectedEOF
	}
	return 0, fmt.Errorf("wxyunpan: 缁紶 %d 娆′粛澶辫触锛堝凡璇?%d 瀛楄妭锛? %w", wxRetryTimes, r.off, lastErr)
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
