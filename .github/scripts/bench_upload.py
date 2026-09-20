import urllib.request, urllib.parse, json, os, time, sys

AL = "http://127.0.0.1:5244"
size_mb = int(os.environ.get("SIZE_MB", "200"))
addition = os.environ["ADDITION"]


def api(method, path, data=None, token=None, raw=False, headers_extra=None, timeout=1800):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = token
    if headers_extra:
        headers.update(headers_extra)
    r = urllib.request.Request(AL + path, headers=headers, method=method)
    if data is not None:
        r.data = data if raw else json.dumps(data).encode()
    try:
        resp = urllib.request.urlopen(r, timeout=timeout)
        return resp.status, resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")
    except Exception as e:
        return -1, repr(e)


# 登录
s, d = api("POST", "/api/auth/login", {"username": "admin", "password": "aaaaa6"})
print("login:", s, d[:200], flush=True)
if s != 200:
    sys.exit(1)
tok = json.loads(d)["data"]["token"]

# 添加豆包存储
s, d = api("POST", "/api/admin/storage/create", {
    "mount_path": "/豆包网盘", "driver": "DoubaoNew", "order": 0,
    "cache_expiration": 30, "status": "work", "disabled": False,
    "addition": addition, "remark": "bench",
}, token=tok)
print("create storage:", s, d[:250], flush=True)

# 验证能列目录（确认凭证有效）
s, d = api("GET", "/api/fs/list?path=" + urllib.parse.quote("/豆包网盘"), token=tok)
print("list /豆包网盘:", s, d[:300], flush=True)

# 上传测速
size = size_mb * 1024 * 1024
payload = os.urandom(size)
name = "/豆包网盘/_bench_%dmb.bin" % size_mb
fn = "_bench_%dmb.bin" % size_mb
print("uploading %d MB ..." % size_mb, flush=True)
t0 = time.time()
s, d = api("PUT", "/api/fs/put", data=payload, token=tok, raw=True,
           headers_extra={"File-Path": urllib.parse.quote(name),
                          "Content-Length": str(len(payload))})
dt = time.time() - t0
print("PUT ->", s, d[:250], "(%.1fs)" % dt, flush=True)

if s == 200 and '"code":200' in d:
    speed = size / dt / 1024 / 1024
    print("===== RESULT: %d MB in %.1fs = %.2f MB/s (%.1f Mbps) =====" % (size_mb, dt, speed, speed * 8), flush=True)
    s2, d2 = api("POST", "/api/fs/remove", {"dir": "/豆包网盘", "names": [fn]}, token=tok)
    print("cleanup:", s2, d2[:150], flush=True)
else:
    print("UPLOAD FAILED", flush=True)
    sys.exit(1)
