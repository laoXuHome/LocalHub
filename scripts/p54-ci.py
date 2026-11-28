#!/usr/bin/env python3
"""P5.4 整体验收：真实 Docker 上的 P1-P4 回归与 Docker 关联检查。

真实 Docker 验证：容器发现、running/stopped/error、关联、重启、故障与恢复。
模拟/单元验证：unknown 容器状态只由 Go/前端单元测试覆盖，本脚本不伪造真实 unknown。
"""
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request
from http.cookiejar import CookieJar
from pathlib import Path

BASE = os.environ.get("P54_BASE", "http://127.0.0.1:18091")
PASSWORD = os.environ["LOCALHUB_ADMIN_PASSWORD"]
OUT = Path(os.environ.get("P54_ARTIFACT_DIR", "ci-artifacts/p54"))
OUT.mkdir(parents=True, exist_ok=True)
STATE = OUT / "p54-state.json"
P53_STATE = Path(os.environ.get("P53_ARTIFACT_DIR", "ci-artifacts/p53")) / "p53-state.json"
jar = CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))


def call(method: str, path: str, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    if data is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with opener.open(req, timeout=15) as resp:
            raw = resp.read()
            return resp.status, json.loads(raw) if raw else None
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        try:
            parsed = json.loads(raw) if raw else None
        except json.JSONDecodeError:
            parsed = raw.decode(errors="replace")
        return exc.code, parsed


def guest(method: str, path: str, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    if data is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read()


def login():
    status, body = call("POST", "/api/auth/login", {"username": "admin", "password": PASSWORD})
    assert status == 200 and body.get("role") == "admin", (status, body)


def dump(name, obj):
    (OUT / name).write_text(json.dumps(obj, ensure_ascii=False, indent=2) + "\n")
    print(name, "written")


def docker(*args):
    return subprocess.run(["docker", *args], check=True, text=True, capture_output=True).stdout.strip()


def docker_ignore(*args):
    subprocess.run(["docker", *args], text=True, capture_output=True, check=False)


def container(containers, name):
    for c in containers["containers"]:
        if c["name"] == name:
            return c
    raise AssertionError("容器不存在：" + name)


def auth_checks():
    status, body = guest("GET", "/api/admin/whoami")
    assert status == 401, status
    status, _ = guest("GET", "/api/bookmarks")
    assert status == 200, status
    status, _ = guest("GET", "/api/auth/status")
    assert status == 200, status
    status, _ = guest("POST", "/api/admin/bookmarks", {"name": "guest-denied", "wan_url": "https://denied.example.com"})
    assert status == 401, status
    dump("p1-auth.json", {"guest_whoami": 401, "guest_bookmarks": 200, "guest_auth_status": 200, "guest_create": 401})
    print("P1 auth PASS")


def bookmark_checks():
    created = call("POST", "/api/admin/bookmarks", {
        "name": "P5.4 回归书签",
        "lan_url": "http://192.168.50.10:5000",
        "wan_url": "https://p54.example.com",
        "description": "P5.4 P2/P4 回归",
        "visibility": "public",
    })
    status, body = created
    assert status == 201, (status, body)
    bid = body["id"]
    status, detail = call("GET", f"/api/admin/bookmarks/{bid}")
    assert status == 200, (status, detail)
    assert detail["lan_url"] == "http://192.168.50.10:5000"
    assert detail["wan_url"] == "https://p54.example.com"
    status, updated = call("PUT", f"/api/admin/bookmarks/{bid}", {"description": "P5.4 P2 更新"})
    assert status == 200 and updated["description"] == "P5.4 P2 更新", (status, updated)
    status, public = call("GET", "/api/bookmarks")
    assert status == 200 and any(b["name"] == "P5.4 回归书签" for b in public["items"])
    status, layout = call("GET", "/api/bookmarks/layout")
    assert status == 200 and "desktop" in layout and "mobile" in layout, layout
    status, saved = call("PUT", "/api/admin/bookmarks/layout", {"desktop": layout["desktop"], "mobile": layout["mobile"]})
    assert status == 200, (status, saved)
    status, again = call("GET", "/api/bookmarks/layout")
    assert status == 200 and len(again["desktop"]) == len(layout["desktop"]), again
    dump("p2-p3-bookmarks-layout.json", {
        "created_id": bid,
        "public_visible": True,
        "layout_desktop": len(layout["desktop"]),
        "layout_mobile": len(layout["mobile"]),
        "lan_wan_preserved": True,
    })
    print("P2 bookmarks + P3 layout PASS")
    return bid


def network_checks():
    status, targets = call("GET", "/api/admin/network/targets")
    assert status == 200 and len(targets["targets"]) == 3, targets
    payload = {"targets": [{"key": t["key"], "address": t["address"], "enabled": t["enabled"]} for t in targets["targets"]]}
    status, saved = call("PUT", "/api/admin/network/targets", payload)
    assert status == 200 and len(saved["targets"]) == 3, (status, saved)
    status, again = call("GET", "/api/admin/network/targets")
    assert status == 200 and again["targets"][0]["address"] == targets["targets"][0]["address"], again
    status, public_targets = call("GET", "/api/network/targets")
    assert status == 200, (status, public_targets)
    status, public_status = call("GET", "/api/network/status")
    assert status == 200, (status, public_status)
    dump("p4-network.json", {
        "targets": len(again["targets"]),
        "enabled_public": len(public_targets.get("targets", [])),
        "public_status": public_status.get("status", "unknown"),
        "lan_wan_selector_source": "bookmark lan_url/wan_url preserved",
    })
    print("P4 network PASS")


def docker_checks():
    status, docker_status = call("GET", "/api/admin/docker/status")
    assert status == 200 and docker_status["status"] == "connected" and docker_status["read_only"] is True, docker_status
    status, containers = call("GET", "/api/admin/docker/containers")
    assert status == 200 and containers["status"] == "connected" and containers["read_only"] is True, containers
    status, links = call("GET", "/api/admin/docker/bookmark-links")
    assert status == 200 and links["status"] == "connected" and links["read_only"] is True, links

    running = container(containers, "p52-running")
    stopped = container(containers, "p52-stopped")
    assert running["status"] == "running", running
    assert stopped["status"] == "stopped", stopped
    for c in containers["containers"]:
        assert len(c["id"]) == 64 and c["image"] and isinstance(c["ports"], list) and isinstance(c["labels"], dict), c

    docker_ignore("rm", "-f", "p54-restarting")
    docker("run", "-d", "--name", "p54-restarting", "--restart=always", "--entrypoint", "false", "alpine:3.20")
    error = None
    for _ in range(20):
        status, containers = call("GET", "/api/admin/docker/containers")
        assert status == 200, (status, containers)
        for c in containers["containers"]:
            if c["name"] == "p54-restarting" and c["status"] == "error":
                error = c
                break
        if error:
            break
        time.sleep(1)
    assert error, "真实 restarting 容器未映射为 error"

    status, links = call("GET", "/api/admin/docker/bookmark-links")
    status_run = next(c["status"] for c in links["containers"] if c["name"] == "p52-running")
    status_stop = next(c["status"] for c in links["containers"] if c["name"] == "p52-stopped")
    status_err = next(c["status"] for c in links["containers"] if c["name"] == "p54-restarting")
    assert (status_run, status_stop, status_err) == ("running", "stopped", "error"), (status_run, status_stop, status_err)
    assert set(b["name"] for b in next(c for c in links["containers"] if c["name"] == "p52-running")["bookmarks"]) == {
        "P5.3 运行关联", "P5.3 共享关联"
    }
    assert set(b["name"] for b in next(c for c in links["containers"] if c["name"] == "p52-stopped")["bookmarks"]) == {"P5.3 停止关联"}
    assert "P5.3 缺失关联" in [m["bookmark_name"] for m in links["missing"]]
    assert links["unlinked"] >= 1
    assert len(next(c for c in links["containers"] if c["name"] == "p52-running")["bookmarks"]) == 2

    dump("p5-docker-four-states-real.json", {
        "running": "p52-running",
        "stopped": "p52-stopped",
        "error": "p54-restarting",
        "unknown": "仅单元测试覆盖，未在真实 Docker 中构造",
        "counts": containers["counts"],
        "multi_bookmark_container": "p52-running",
        "missing_and_unlinked": True,
    })
    print("P5 Docker running/stopped/error + associations PASS")
    return {"error_id": error["id"]}


def save_state(bid, error_id):
    STATE.write_text(json.dumps({"bookmark_id": bid, "error_id": error_id}) + "\n")


def pre():
    login()
    auth_checks()
    bid = bookmark_checks()
    network_checks()
    extra = docker_checks()
    save_state(bid, extra["error_id"])
    dump("p5.4-pre-summary.json", {"status": "PASS", "coverage": "P1-P4 + Docker real", "unknown": "unit simulation only"})
    print("P5.4 pre PASS")


def outage():
    login()
    status, docker_status = call("GET", "/api/admin/docker/status")
    assert status == 200 and docker_status["status"] == "connection_failed", docker_status
    status, containers = call("GET", "/api/admin/docker/containers")
    assert status == 200 and containers["status"] == "connection_failed" and containers["containers"] == [], containers
    status, links = call("GET", "/api/admin/docker/bookmark-links")
    assert status == 200 and links["status"] == "connection_failed", links
    assert links["containers"] == [] and links["missing"] == [], "故障时不得判定容器不存在"

    status, bookmarks = call("GET", "/api/admin/bookmarks")
    assert status == 200 and bookmarks["items"], bookmarks
    linked = {b["name"]: b["docker_container"] for b in bookmarks["items"] if b["name"].startswith("P5.3 ")}
    assert linked["P5.3 运行关联"] and linked["P5.3 停止关联"] and linked["P5.3 共享关联"], linked
    status, public = guest("GET", "/api/bookmarks")
    assert status == 200, status
    status, auth = guest("GET", "/api/auth/status")
    assert status == 200, status
    status, health = guest("GET", "/health")
    assert status == 200, status
    dump("p5.4-outage.json", {
        "docker_status": docker_status["status"],
        "containers": 0,
        "missing": 0,
        "associations_kept": True,
        "public_bookmarks": 200,
        "auth_status": 200,
        "health": 200,
    })
    print("P5.4 outage + P1/P2/P3 unaffected PASS")


def final():
    login()
    state = json.loads(STATE.read_text())
    p53 = json.loads(P53_STATE.read_text())
    status, containers = call("GET", "/api/admin/docker/containers")
    assert status == 200 and containers["status"] == "connected", containers
    new_stop = container(containers, "p52-stopped")
    assert new_stop["id"] != p53["stop_id"], (new_stop["id"], p53["stop_id"])
    status, links = call("GET", "/api/admin/docker/bookmark-links")
    linked_new = next(c for c in links["containers"] if c["name"] == "p52-stopped")
    assert linked_new["bookmarks"] == []
    old = next(m for m in links["missing"] if m["bookmark_name"] == "P5.3 停止关联")
    assert old["container_id"] == p53["stop_id"]
    assert old["container_id"] != new_stop["id"]

    status, bookmarks = call("GET", "/api/admin/bookmarks")
    assert status == 200
    status, public = call("GET", "/api/bookmarks")
    assert status == 200, status
    status, auth = call("GET", "/api/auth/status")
    assert status == 200, status
    status, network = call("GET", "/api/network/status")
    assert status == 200, status

    status, _ = call("DELETE", f"/api/admin/bookmarks/{state['bookmark_id']}")
    assert status == 200, status
    status, public = call("GET", "/api/bookmarks")
    assert all(b["name"] != "P5.4 回归书签" for b in public["items"])
    docker_ignore("rm", "-f", "p54-restarting")

    dump("p5.4-same-name-final.json", {
        "old_id": p53["stop_id"],
        "new_id": new_stop["id"],
        "new_bookmarks": 0,
        "old_missing_preserved": True,
        "P1_auth": 200,
        "P3_home_api": 200,
        "P4_network": 200,
        "regression_bookmark_removed": True,
    })
    print("P5.4 same-name + final regression PASS")


PHASES = {"pre": pre, "outage": outage, "final": final}

if __name__ == "__main__":
    if len(sys.argv) != 2 or sys.argv[1] not in PHASES:
        raise SystemExit("usage: p54-ci.py " + "|".join(PHASES))
    PHASES[sys.argv[1]]()
