package wxyunpan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
)

// Offline 实现 AList 的离线下载接口（driver.Offline）。
// 前端通过 /api/fs/other 传 method="offline_download"，data 为链接（magnet/ed2k/http）。
func (d *WxYunPan) Offline(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	switch args.Method {
	case "offline", "offline_download":
		url := parseOfflineURL(args.Data)
		if url == "" {
			return nil, fmt.Errorf("离线下载链接为空")
		}
		return d.addOffline(ctx, url)
	case "offline_download_list", "offline_list":
		return d.offlineList(ctx)
	default:
		return nil, errs.NotSupport
	}
}

// parseOfflineURL 兼容 data 为字符串或 {url}/{urls} 结构
func parseOfflineURL(data interface{}) string {
	switch v := data.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		if u, ok := v["url"].(string); ok {
			return strings.TrimSpace(u)
		}
		if us, ok := v["urls"].([]interface{}); ok && len(us) > 0 {
			if u, ok := us[0].(string); ok {
				return strings.TrimSpace(u)
			}
		}
	}
	// 兜底：尝试 JSON 解析
	if b, err := json.Marshal(data); err == nil {
		var m map[string]interface{}
		if json.Unmarshal(b, &m) == nil {
			if u, ok := m["url"].(string); ok {
				return strings.TrimSpace(u)
			}
		}
	}
	return ""
}

// addOffline 提交离线任务：POST /magnet/upload {url}
func (d *WxYunPan) addOffline(ctx context.Context, url string) (interface{}, error) {
	raw, err := d.request(ctx, "POST", "/magnet/upload", map[string]interface{}{"url": url})
	if err != nil {
		return nil, err
	}
	var task offlineTask
	_ = json.Unmarshal(raw, &task)
	return task, nil
}

// offlineList 列出离线任务：GET /task/list
func (d *WxYunPan) offlineList(ctx context.Context) (interface{}, error) {
	raw, err := d.request(ctx, "GET", "/task/list", nil)
	if err != nil {
		return nil, err
	}
	var tasks []offlineTask
	if err := json.Unmarshal(raw, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

var _ interface {
	Offline(ctx context.Context, args model.OtherArgs) (interface{}, error)
} = (*WxYunPan)(nil)
