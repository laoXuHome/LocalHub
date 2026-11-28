#!/usr/bin/env python3
"""P5.3 容器关联 CI 验收：真实 Docker fixture，不使用模拟数据。"""
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request
from http.cookiejar import CookieJar
from pathlib import Path

BASE = os.environ.get("P53_BASE", "http://127.0.0.1:18090")
PASSWORD = os.environ["LOCALHUB_ADMIN_PASSWORD"]
OUT = Path(os.environ.get("P53_ARTIFACT_DIR", "ci-artifacts/p53"))
OUT.mkdir(parents=True, exist_ok=True)
jar = CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
STATE = OUT / "p53-state.json"


def call(method: str, path: str, body=None, auth=True):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    if data is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with opener.open(req, timeout=10) as resp:
            raw = resp.read()
            return resp.status, json.loads(raw) if raw else None
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        try:
            parsed = json.loads(raw) if raw else None
        except json.JSONDecodeError:
            parsed = raw.decode(errors="replace")
        return exc.code, parsed


def login():
    status, body = call("POST", "/api/auth/login", {"username": "admin", "password": PASSWORD})
    assert status == 200 and body["role"] == "admin", (status, body)


def dump(name, obj):
    (OUT / name).write_text(json.dumps(obj, ensure_ascii=False, indent=2) + "\n")
    print(name, "written")


def api_links():
    status, body = call("GET", "/api/admin/docker/bookmark-links")
    assert status == 200, (status, body)
    return body


def bookmarks():
    status, body = call("GET", "/api/admin/bookmarks")
    assert status == 200, (status, body)
    return body["items"]


def by_name(links, name):
    for c in links["containers"]:
        if c["name"] == name:
            return c
    raise AssertionError("容器不存在：" + name)


def create(name, container):
    status, body = call("POST", "/api/admin/bookmarks", {
        "name": name,
        "wan_url": "https://p53.example.com",
        "docker_container": container,
    })
    assert status == 201, (status, body)
    return body["id"]


def patch(bookmark_id, container):
    status, updated = call("PUT", f"/api/admin/bookmarks/{bookmark_id}", {"docker_container": container})
    assert status == 200, (status, updated)


def docker(*args):
    return subprocess.run(["docker", *args], check=True, text=True, capture_output=True).stdout.strip()


def setup():
    login()
    guest = urllib.request.Request(BASE + "/api/admin/docker/bookmark-links")
    try:
        urllib.request.urlopen(guest, timeout=10)
        raise AssertionError("访客读取关联接口不应成功")
    except urllib.error.HTTPError as exc:
        assert exc.code == 401, exc.code

    status, containers = call("GET", "/api/admin/docker/containers")
    assert status == 200 and containers["status"] == "connected", containers
    dump("containers-before.json", containers)
    ids = {c["name"]: c["id"] for c in containers["containers"]}
    run_id, stop_id = ids["p52-running"], ids["p52-stopped"]
    assert len(run_id) == 64 and len(stop_id) == 64

    linked_run = create("P5.3 运行关联", run_id)
    linked_stop = create("P5.3 停止关联", stop_id)
    linked_shared = create("P5.3 共享关联", run_id)
    linked_missing = create("P5.3 缺失关联", "a" * 64)
    plain = create("P5.3 未关联", "")
    assert len({linked_run, linked_stop, linked_shared, linked_missing, plain}) == 5

    links = api_links()
    dump("links-setup.json", links)
    assert links["status"] == "connected" and links["read_only"] is True
    assert {b["name"] for b in by_name(links, "p52-running")["bookmarks"]} == {"P5.3 运行关联", "P5.3 共享关联"}
    assert by_name(links, "p52-running")["status"] == "running"
    assert {b["name"] for b in by_name(links, "p52-stopped")["bookmarks"]} == {"P5.3 停止关联"}
    assert by_name(links, "p52-stopped")["status"] == "stopped"
    assert "P5.3 缺失关联" in [m["bookmark_name"] for m in links["missing"]]
    assert links["unlinked"] >= 1

    patch(linked_run, stop_id)
    links = api_links()
    dump("links-changed.json", links)
    assert {b["name"] for b in by_name(links, "p52-running")["bookmarks"]} == {"P5.3 共享关联"}
    assert {b["name"] for b in by_name(links, "p52-stopped")["bookmarks"]} == {"P5.3 停止关联", "P5.3 运行关联"}
    patch(linked_run, run_id)

    patch(plain, stop_id)
    links = api_links()
    assert [b["name"] for b in by_name(links, "p52-stopped")["bookmarks"]].count("P5.3 未关联") == 1
    patch(plain, "")
    links = api_links()
    dump("links-unlinked.json", links)
    assert all(b["name"] != "P5.3 未关联" for c in links["containers"] for b in c["bookmarks"])

    state = {
        "run_id": run_id,
        "stop_id": stop_id,
        "linked_run": linked_run,
        "linked_stop": linked_stop,
        "linked_shared": linked_shared,
        "linked_missing": linked_missing,
        "plain": plain,
    }
    STATE.write_text(json.dumps(state, indent=2) + "\n")
    print("P5.3 setup PASS")


def after_restart():
    login()
    state = json.loads(STATE.read_text())
    links = api_links()
    dump("links-after-restart.json", links)
    bms = bookmarks()
    linked = {b["name"]: b["docker_container"] for b in bms if b["name"].startswith("P5.3 ")}
    assert linked["P5.3 运行关联"] == state["run_id"], linked
    assert linked["P5.3 停止关联"] == state["stop_id"], linked
    assert linked["P5.3 共享关联"] == state["run_id"], linked
    assert {b["name"] for b in by_name(links, "p52-running")["bookmarks"]} == {"P5.3 运行关联", "P5.3 共享关联"}
    print("P5.3 persistence PASS")


def status_change():
    login()
    docker("stop", "p52-running")
    links = api_links()
    dump("links-container-stopped.json", links)
    assert by_name(links, "p52-running")["status"] == "stopped"
    docker("start", "p52-running")
    for _ in range(20):
        links = api_links()
        if by_name(links, "p52-running")["status"] == "running":
            break
        import time
        time.sleep(1)
    dump("links-container-restarted.json", links)
    assert by_name(links, "p52-running")["status"] == "running"
    print("P5.3 status change PASS")


def outage():
    login()
    links = api_links()
    dump("links-outage.json", links)
    assert links["status"] == "connection_failed", links
    assert links["containers"] == []
    assert links["missing"] == [], "Docker 故障时不得判定容器不存在"
    bms = bookmarks()
    linked = {b["name"]: b["docker_container"] for b in bms if b["name"].startswith("P5.3 ")}
    assert linked["P5.3 运行关联"] and linked["P5.3 停止关联"], linked
    print("P5.3 outage keeps associations PASS")


def recovery():
    login()
    links = api_links()
    dump("links-recovered.json", links)
    assert links["status"] == "connected"
    assert "P5.3 停止关联" in [b["name"] for b in by_name(links, "p52-stopped")["bookmarks"]]
    print("P5.3 recovery PASS")


def removed_and_recreated():
    login()
    state = json.loads(STATE.read_text())
    docker("rm", "-f", "p52-stopped")
    links = api_links()
    dump("links-container-removed.json", links)
    assert links["status"] == "connected"
    assert "P5.3 停止关联" in [m["bookmark_name"] for m in links["missing"]]
    # 同名但新 ID 的容器：旧关联必须继续指向旧 ID，不能被新容器接管
    docker("create", "--name", "p52-stopped", "--entrypoint", "true", "alpine:3.20")
    new_id = docker("inspect", "-f", "{{.Id}}", "p52-stopped")
    assert new_id != state["stop_id"]
    links = api_links()
    dump("links-same-name-new-id.json", links)
    assert by_name(links, "p52-stopped")["id"] == new_id
    assert by_name(links, "p52-stopped")["bookmarks"] == []
    assert "P5.3 停止关联" in [m["bookmark_name"] for m in links["missing"]]
    assert next(m for m in links["missing"] if m["bookmark_name"] == "P5.3 停止关联")["container_id"] == state["stop_id"]
    (OUT / "same-name-old-new-ids.txt").write_text(f"old={state['stop_id']}\nnew={new_id}\n")
    print("P5.3 removed and same-name new ID PASS")


PHASES = {
    "setup": setup,
    "after-restart": after_restart,
    "status": status_change,
    "outage": outage,
    "recovery": recovery,
    "remove": removed_and_recreated,
}

if __name__ == "__main__":
    if len(sys.argv) != 2 or sys.argv[1] not in PHASES:
        raise SystemExit("usage: p53-ci.py " + "|".join(PHASES))
    PHASES[sys.argv[1]]()
