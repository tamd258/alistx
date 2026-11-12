package wxyunpan

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	Account     string `json:"account" required:"true" help:"手机号或邮箱"`
	Password    string `json:"password" required:"true"`
	SessionKey  string `json:"session_key" required:"false" help:"可选：直接填登录后的 sessionKey，优先级高于账号密码"`
	RootFolder  string `json:"root_folder" required:"false" default:"0" help:"根目录 id，默认 0 表示根目录"`
}

var config = driver.Config{
	Name:        "WxYunPan",
	LocalSort:   true,
	OnlyLocal:   false,
	OnlyProxy:   false,
	NoCache:     false,
	NoUpload:    false,
	NeedMs:      false,
	DefaultRoot: "0",
	CheckStatus: false,
	Alert:       "无限云盘（wxyunpan）：后端为 PikPak，支持磁力/ed2k/http 离线下载。",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &WxYunPan{}
	})
}
