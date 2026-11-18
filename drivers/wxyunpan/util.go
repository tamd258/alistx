package wxyunpan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/op"
)

const (
	apiBase = "https://api.xbapi.com/v1/app"
	ua      = "okhttp/3.12.13"
)

// 閫氱敤璇锋眰锛氳繑鍥炲師濮?JSON body銆傞亣鍒?session 杩囨湡(60001)/鏈櫥褰?60002) 浼氳嚜鍔ㄩ噸鏂扮櫥褰曞苟閲嶈瘯涓€娆°€?func (d *WxYunPan) request(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	return d.requestRetry(ctx, method, path, body, true)
}

func (d *WxYunPan) requestRetry(ctx context.Context, method, path string, body interface{}, allowRelogin bool) (json.RawMessage, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	} else if method == http.MethodPost {
		reqBody = bytes.NewReader([]byte("{}"))
	}

	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("platform", "APP")
	req.Header.Set("thirdSession", d.sessionKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := wxHttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("瑙ｆ瀽鍝嶅簲澶辫触: %s", truncate(string(raw), 200))
	}
	// 60001 = 鐧诲綍杩囨湡锛?0002 = 鏈櫥褰?session 澶辨晥 鈫?娓呯┖鏃?sessionKey 閲嶆柊鐧诲綍鍚庨噸璇曚竴娆?	if (wrapper.Code == 60001 || wrapper.Code == 60002) && allowRelogin {
		d.sessionKey = ""
		if err := d.login(ctx); err == nil {
			return d.requestRetry(ctx, method, path, body, false)
		}
	}
	if wrapper.Code != 200 && wrapper.Code != 5555 {
		return nil, fmt.Errorf("%s (code=%d) %s", path, wrapper.Code, wrapper.Msg)
	}
	return wrapper.Data, nil
}

func (d *WxYunPan) login(ctx context.Context) error {
	acc := d.Addition.Account
	pw := d.Addition.Password
	if acc == "" || pw == "" {
		return fmt.Errorf("鏈厤缃处鍙锋垨瀵嗙爜")
	}
	// 閭蹇呴』鐢?email 瀛楁锛涙墜鏈哄彿鐢?account/phone
	bodies := []map[string]interface{}{}
	if contains(acc, "@") {
		bodies = append(bodies,
			map[string]interface{}{"email": acc, "account": acc, "password": pw, "inviteCode": ""},
			map[string]interface{}{"account": acc, "phone": acc, "password": pw, "inviteCode": ""},
		)
	} else {
		bodies = append(bodies,
			map[string]interface{}{"account": acc, "phone": acc, "password": pw, "inviteCode": ""},
			map[string]interface{}{"email": acc, "account": acc, "password": pw, "inviteCode": ""},
		)
	}

	var lastErr error
	for _, b := range bodies {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/m/user/login", mustJson(b))
		req.Header.Set("User-Agent", ua)
		req.Header.Set("platform", "APP")
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		resp, err := wxHttpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var w struct {
			Code int       `json:"code"`
			Msg  string    `json:"msg"`
			Data loginData `json:"data"`
		}
		if err := json.Unmarshal(raw, &w); err != nil {
			lastErr = err
			continue
		}
		if w.Code == 200 && w.Data.SessionKey != "" {
			d.sessionKey = w.Data.SessionKey
			// 鎸佷箙鍖栧埌 driver storage锛岄伩鍏嶆瘡娆￠兘鐧诲綍
			op.MustSaveDriverStorage(d)
			return nil
		}
		lastErr = fmt.Errorf("鐧诲綍澶辫触: %s (code=%d)", w.Msg, w.Code)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("鐧诲綍澶辫触")
	}
	return lastErr
}

func mustJson(v interface{}) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func nowUnix() int64 { return time.Now().Unix() }
