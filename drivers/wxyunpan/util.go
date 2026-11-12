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

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/op"
)

const (
	apiBase = "https://api.xbapi.com/v1/app"
	ua      = "okhttp/3.12.13"
)

// 通用请求：返回原始 JSON body。遇到 session 过期(60001)/未登录(60002) 会自动重新登录并重试一次。
func (d *WxYunPan) request(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
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

	resp, err := base.HttpClient.Do(req)
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
		return nil, fmt.Errorf("解析响应失败: %s", truncate(string(raw), 200))
	}
	// 60001 = 登录过期，60002 = 未登录/session 失效 → 清空旧 sessionKey 重新登录后重试一次
	if (wrapper.Code == 60001 || wrapper.Code == 60002) && allowRelogin {
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
		return fmt.Errorf("未配置账号或密码")
	}
	// 邮箱必须用 email 字段；手机号用 account/phone
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
		resp, err := base.HttpClient.Do(req)
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
			// 持久化到 driver storage，避免每次都登录
			op.MustSaveDriverStorage(d)
			return nil
		}
		lastErr = fmt.Errorf("登录失败: %s (code=%d)", w.Msg, w.Code)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("登录失败")
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
