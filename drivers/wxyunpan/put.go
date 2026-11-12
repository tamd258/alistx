package wxyunpan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
)

// Put 上传文件：multipart POST /upload，字段名 file
func (d *WxYunPan) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// 可选：附带父目录 id（若接口支持）
	parentID := dstDir.GetID()
	if parentID == "" {
		parentID = "0"
	}
	_ = w.WriteField("parentId", parentID)

	fw, err := w.CreateFormFile("file", stream.GetName())
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(fw, stream); err != nil {
		return nil, err
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/upload", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("platform", "APP")
	req.Header.Set("thirdSession", d.sessionKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := base.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var wrapper struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("上传响应解析失败: %s", truncate(string(raw), 200))
	}
	if wrapper.Code != 200 {
		return nil, fmt.Errorf("上传失败: %s (code=%d)", wrapper.Msg, wrapper.Code)
	}
	// 返回的文件对象结构未知，尽量解析常见字段
	var f wxFile
	_ = json.Unmarshal(wrapper.Data, &f)
	if f.Name == "" {
		f.Name = stream.GetName()
	}
	return f.toObj(), nil
}
