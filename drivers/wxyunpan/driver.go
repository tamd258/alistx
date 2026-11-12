package wxyunpan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
)

type WxYunPan struct {
	model.Storage
	Addition
	sessionKey string
}

func (d *WxYunPan) Config() driver.Config {
	return config
}

func (d *WxYunPan) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *WxYunPan) Init(ctx context.Context) error {
	// 优先使用手动填写的 sessionKey，否则用账号密码登录
	if d.Addition.SessionKey != "" {
		d.sessionKey = d.Addition.SessionKey
		return nil
	}
	return d.login(ctx)
}

func (d *WxYunPan) Drop(ctx context.Context) error {
	return nil
}

// List 列目录：POST /disk/fileList
func (d *WxYunPan) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	parentId := dir.GetID()
	if parentId == "" {
		parentId = "0"
	}
	var all []model.Obj
	const pageSize = 200
	for current := 1; current <= 50; current++ {
		body := map[string]interface{}{
			"parentId": parentId,
			"current":  current,
			"size":     pageSize,
		}
		raw, err := d.request(ctx, "POST", "/disk/fileList", body)
		if err != nil {
			return nil, err
		}
		var data diskFileListData
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
		for _, f := range data.Files {
			all = append(all, f.toObj())
		}
		if len(data.Files) < pageSize || current >= data.Pages {
			break
		}
	}
	return all, nil
}

// Link 取直链：POST /magnet/preparePlay {fileId, shareId}
func (d *WxYunPan) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	// obj.ID 形如 "fileId|shareId"（文件夹则为纯 id）
	fileID := file.GetID()
	shareID := ""
	if i := strings.Index(fileID, "|"); i >= 0 {
		shareID = fileID[i+1:]
		fileID = fileID[:i]
	}
	body := map[string]interface{}{
		"fileId": fileID,
	}
	if shareID != "" {
		body["shareId"] = shareID
	}
	raw, err := d.request(ctx, "POST", "/magnet/preparePlay", body)
	if err != nil {
		return nil, err
	}
	var pd preparePlayData
	if err := json.Unmarshal(raw, &pd); err != nil {
		return nil, err
	}
	url := pd.WebContentLink
	if url == "" {
		url = pd.DefaultPlayUrl
	}
	if url == "" {
		url = pd.OriginPlayUrl
	}
	if url == "" {
		return nil, fmt.Errorf("未获取到直链")
	}
	return &model.Link{URL: url}, nil
}

// MakeDir 新建文件夹：POST /disk/addFolder
func (d *WxYunPan) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	parentId := parentDir.GetID()
	if parentId == "" {
		parentId = "0"
	}
	body := map[string]interface{}{
		"parentId": parentId,
		"name":     dirName,
	}
	if _, err := d.request(ctx, "POST", "/disk/addFolder", body); err != nil {
		return nil, err
	}
	return &model.Object{Name: dirName, IsFolder: true}, nil
}

// Move 移动：POST /disk/moveFile {id, parentId}
func (d *WxYunPan) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	dstID := dstDir.GetID()
	if dstID == "" {
		dstID = "0"
	}
	body := map[string]interface{}{
		"id":       srcObj.GetID(),
		"parentId": dstID,
	}
	if _, err := d.request(ctx, "POST", "/disk/moveFile", body); err != nil {
		return nil, err
	}
	return srcObj, nil
}

// Rename 重命名：文件夹 /disk/rename，文件 /file/rename
func (d *WxYunPan) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	path := "/file/rename"
	if srcObj.IsDir() {
		path = "/disk/rename"
	}
	body := map[string]interface{}{
		"id":   srcObj.GetID(),
		"name": newName,
	}
	if _, err := d.request(ctx, "POST", path, body); err != nil {
		return nil, err
	}
	return &model.Object{ID: srcObj.GetID(), Name: newName, IsFolder: srcObj.IsDir()}, nil
}

// Remove 删除：文件夹 /disk/del，文件 /file/addDel
func (d *WxYunPan) Remove(ctx context.Context, obj model.Obj) error {
	path := "/file/addDel"
	if obj.IsDir() {
		path = "/disk/del"
	}
	_, err := d.request(ctx, "POST", path, map[string]interface{}{"id": obj.GetID()})
	return err
}

// Put 上传：multipart POST /upload —— 见 put.go 实现
// 见 put.go

var _ driver.Driver = (*WxYunPan)(nil)
