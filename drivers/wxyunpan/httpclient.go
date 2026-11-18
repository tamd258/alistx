package wxyunpan

import (
	"context"
	"net"
	"net/http"
	"time"
)

// 本驱动所有出站请求（api.xbapi.com + PikPak 直链）都走这个客户端：
//  1. 不走系统/环境代理（Proxy: nil）——本机代理是给 Google 之类用的，
//     PikPak 与 xbapi 国内直连更快，走代理反而会挂住不放数据；
//  2. 每次 Read 都设读超时（timeoutConn）——PikPak 的连接会「假活」：
//     TCP 还在、一个字节都不给，不设读超时就永远阻塞；
//     超时后返回 error，上层的 wxResumeReader 会按 Range 续传。
var wxHttpClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 nil,
		DialContext:           wxDialContext,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

const wxReadIdleTimeout = 90 * time.Second

var wxDialer = &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}

func wxDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	c, err := wxDialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	return &wxTimeoutConn{Conn: c, idle: wxReadIdleTimeout}, nil
}

type wxTimeoutConn struct {
	net.Conn
	idle time.Duration
}

func (c *wxTimeoutConn) Read(p []byte) (int, error) {
	_ = c.Conn.SetReadDeadline(time.Now().Add(c.idle))
	return c.Conn.Read(p)
}

func (c *wxTimeoutConn) Write(p []byte) (int, error) {
	_ = c.Conn.SetWriteDeadline(time.Now().Add(c.idle))
	return c.Conn.Write(p)
}
