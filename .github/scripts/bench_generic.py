import urllib.request, urllib.parse, json, os, time, sys

AL = "http://127.0.0.1:5244"
driver = os.environ.get("DRIVER", "GoogleDrive")
addition = os.environ["ADDITION"]
mount = os.environ.get("MOUNT_PATH", "/GoogleDrive2")
size_mb = int(os.environ.get("SIZE_MB", "200"))
do_up = os.environ.get("DO_UPLOAD", "1") == "1"
do_down = os.environ.get("DO_DOWNLOAD", "1") == "1"


def api(method, path, data=None, token=None, raw=False, headers_extra=None, timeout=3600):
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


s, d = api("POST", "/api/auth/login", {"username": "admin", "password": "aaaaa6"})
print("login:", s, flush=True)
tok = json.loads(d)["data"]["token"]

s, d = api("POST", "/api/admin/storage/create", {
    "mount_path": mount, "driver": driver, "order": 0,
    "cache_expiration": 30, "status": "work", "disabled": False,
    "addition": addition, "remark": "bench",
}, token=tok)
print("create storage:", s, d[:250], flush=True)

s, d = api("GET", "/api/fs/list?path=" + urllib.parse.quote(mount), token=tok)
print("list %s: %s %s" % (mount, s, d[:250]), flush=True)

fn = "_bench_%dmb.bin" % size_mb
name = mount + "/" + fn
size = size_mb * 1024 * 1024

if do_up:
    payload = os.urandom(size)
    print("uploading %d MB ..." % size_mb, flush=True)
    t0 = time.time()
    s, d = api("PUT", "/api/fs/put", data=payload, token=tok, raw=True,
               headers_extra={"File-Path": urllib.parse.quote(name),
                              "Content-Length": str(len(payload))})
    dt = time.time() - t0
    print("UPLOAD -> %s %s (%.1fs)" % (s, d[:200], dt), flush=True)
    if s == 200 and '"code":200' in d:
        print("===== UPLOAD RESULT: %d MB in %.1fs = %.2f MB/s (%.1f Mbps) =====" %
              (size_mb, dt, size / dt / 1024 / 1024, size / dt / 1024 / 1024 * 8), flush=True)
    else:
        print("UPLOAD FAILED", flush=True)

if do_down:
    s, d = api("POST", "/api/fs/get", {"path": name}, token=tok)
    raw_url = ""
    try:
        raw_url = json.loads(d)["data"].get("raw_url") or ""
    except Exception:
        pass
    print("get raw_url:", s, raw_url[:120], flush=True)
    if raw_url.startswith("/"):
        raw_url = AL + raw_url
    if raw_url:
        print("downloading ...", flush=True)
        t0 = time.time()
        req = urllib.request.Request(raw_url, headers={"User-Agent": "curl/8"})
        got = 0
        with urllib.request.urlopen(req, timeout=3600) as resp:
            while True:
                chunk = resp.read(1024 * 512)
                if not chunk:
                    break
                got += len(chunk)
        dt = time.time() - t0
        print("===== DOWNLOAD RESULT: %.1f MB in %.1fs = %.2f MB/s (%.1f Mbps) =====" %
              (got / 1024 / 1024, dt, got / dt / 1024 / 1024, got / dt / 1024 / 1024 * 8), flush=True)
    else:
        print("DOWNLOAD FAILED (no raw_url)", flush=True)

# 清理
s, d = api("POST", "/api/fs/remove", {"dir": mount, "names": [fn]}, token=tok)
print("cleanup:", s, d[:150], flush=True)
