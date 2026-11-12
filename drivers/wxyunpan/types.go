package wxyunpan

import (
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
)

// ===== 通用响应 =====

type baseResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data []byte `json:"-"` // 不使用，各接口单独解析
}

// ===== 登录 =====

type loginData struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	NickName   string `json:"nickName"`
	SessionKey string `json:"sessionKey"`
	Points     int64  `json:"points"`
	Vip        any    `json:"vip"`
}

// ===== 文件对象（/disk/fileList 的 files[] 元素）=====

type wxFile struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`       // 数据库 id（数字串）
	ParentID    string `json:"parentId"` // 父目录 id
	Name        string `json:"name"`
	FileSize    string `json:"fileSize"`
	Size        string `json:"size"`
	MimeType    string `json:"mimeType"`
	FileID      string `json:"fileId"`  // PikPak fileId（文件夹为 ""）
	ShareID     string `json:"shareId"` // 分享文件必需
	FileCategory string `json:"fileCategory"`
	Platform    string `json:"platform"`
	FolderType  string `json:"folderType"` // disk / share
	Type        string `json:"type"`       // "1"=文件 "2"=文件夹
	Status      string `json:"status"`
	Message     string `json:"message"`
	Progress    int    `json:"progress"`
	Hash        string `json:"hash"`
	CreateTime  string `json:"createTime"` // "11-12 06:04" 或 "2026-11-12 06:04"
	UpdateTime  string `json:"updateTime"`
	Poster      string `json:"poster"`
	IconLink    string `json:"iconLink"`
}

type diskFileListData struct {
	Total         int      `json:"total"`
	Size          int      `json:"size"`
	Current       int      `json:"current"`
	Pages         int      `json:"pages"`
	NextPageToken string   `json:"nextPageToken"`
	Files         []wxFile `json:"files"`
}

// ===== 播放/直链（/magnet/preparePlay 的 data）=====

type preparePlayData struct {
	DefaultPlayUrl string `json:"defaultPlayUrl"`
	OriginPlayUrl  string `json:"originPlayUrl"`
	WebContentLink string `json:"webContentLink"`
	Name           string `json:"name"`
	FileID         string `json:"fileId"`
	Size           string `json:"size"`
	MimeType       string `json:"mimeType"`
	IsVip          bool   `json:"isVip"`
}

// ===== 离线任务（/magnet/upload 的 data 及 /task/list 元素）=====

type offlineTask struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	TaskID   string `json:"taskId"`
	Type     string `json:"type"` // offline
	FileID   string `json:"fileId"`
	FileName string `json:"fileName"`
	Name     string `json:"name"`
	FileSize string `json:"fileSize"`
	Url      string `json:"url"`
	Hash     string `json:"hash"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	Progress int    `json:"progress"`
	Platform string `json:"platform"`
	CreateAt string `json:"createTime"`
}

// ===== 转换 =====

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	layouts := []string{"2006-01-02 15:04:05", "01-02 15:04", "2006-01-02 15:04"}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (f wxFile) isDir() bool {
	return f.Type == "2" || f.Kind == "drive#folder"
}

func (f wxFile) toObj() model.Obj {
	size, _ := strconv.ParseInt(f.FileSize, 10, 64)
	if size == 0 {
		size, _ = strconv.ParseInt(f.Size, 10, 64)
	}
	// 文件用 "fileId|shareId" 作 ID（preparePlay 需要两者），文件夹用数据库 id（fileList 需要）
	var id string
	if f.isDir() {
		id = f.ID
	} else {
		id = f.FileID
		if f.ShareID != "" {
			id = f.FileID + "|" + f.ShareID
		}
	}
	if id == "" {
		id = f.ID
	}
	obj := &model.Object{
		ID:       id,
		Name:     f.Name,
		Size:     size,
		IsFolder: f.isDir(),
		Modified: parseTime(f.UpdateTime),
		Ctime:    parseTime(f.CreateTime),
	}
	return obj
}

// 拆分 ID 前缀，区分 文件夹(数据库id) 和 文件(PikPak fileId) 无法从单一字符串判断，
// 故用 GetID 原样保存，Link/Put 时按上下文处理。
