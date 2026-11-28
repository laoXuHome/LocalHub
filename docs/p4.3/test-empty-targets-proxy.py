#!/usr/bin/env python3
"""Local-only screenshot harness: serves web/dist and forwards /api to the test backend
with an isolated admin test session. Not part of the product."""
import http.server
import http.cookiejar
import socketserver
import urllib.request
import urllib.error
import os

DIST = "/home/user/Doubao/chats/38456931691162882/LocalHub-P0/web/dist"
BACKEND = "http://127.0.0.1:11030"
JAR = "/tmp/p41-cookies.txt"


def load_cookie():
    jar = http.cookiejar.MozillaCookieJar(JAR)
    jar.load(ignore_discard=True, ignore_expires=True)
    return "; ".join(f"{c.name}={c.value}" for c in jar)


class Handler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=DIST, **kwargs)

    def log_message(self, fmt, *args):
        pass

    def _proxy(self):
        path = self.path
        body = None
        if self.command in ("POST", "PUT", "PATCH"):
            length = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(length) if length else b""
        req = urllib.request.Request(BACKEND + path, data=body, method=self.command)
        for key in ("Content-Type", "Accept"):
            if self.headers.get(key):
                req.add_header(key, self.headers[key])
        req.add_header("Host", "127.0.0.1:11030")
        req.add_header("Origin", "http://127.0.0.1:11030")
        req.add_header("Cookie", load_cookie())
        try:
            resp = urllib.request.urlopen(req, timeout=30)
            status = resp.status
            data = resp.read()
            ctype = resp.headers.get("Content-Type", "application/octet-stream")
        except urllib.error.HTTPError as e:
            status = e.code
            data = e.read()
            ctype = e.headers.get("Content-Type", "application/json")
        self.send_response(status)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path.split("?")[0] == "/api/network/targets":
            data = b'{"targets":[]}'
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
            return
        if self.path.startswith("/api/") or self.path.startswith("/health"):
            return self._proxy()
        return super().do_GET()

    def do_PUT(self):
        return self._proxy()

    def do_POST(self):
        return self._proxy()


class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True


if __name__ == "__main__":
    with Server(("127.0.0.1", 5444), Handler) as httpd:
        httpd.serve_forever()
