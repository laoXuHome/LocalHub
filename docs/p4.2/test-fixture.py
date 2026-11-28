#!/usr/bin/env python3
"""P4.2 local-only HTTP fixtures. Not part of the product."""
import http.server
import socketserver
import threading
import time

LOCK = threading.Lock()
COUNTS = {}
LOG = "/tmp/p42-fixtures-requests.log"

FIXTURES = [
    ("127.0.0.1", 80, "ok-router"),
    ("127.0.0.3", 80, "ok-bypass"),
    ("127.0.0.4", 80, "ok-nas"),
    ("127.0.0.6", 80, "error"),
    ("127.0.0.7", 80, "slow"),
    ("127.0.0.8", 80, "redirect"),
    ("127.0.0.9", 80, "huge"),
    ("127.0.0.10", 80, "count"),
]


class Handler(http.server.BaseHTTPRequestHandler):
    mode = "ok"
    count_key = ""

    def log_message(self, fmt, *args):
        pass

    def do_GET(self):
        if self.path == "/__count":
            with LOCK:
                value = COUNTS.get(self.count_key, 0)
            body = str(value).encode()
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        with LOCK:
            if self.mode == "count":
                COUNTS[self.count_key] = COUNTS.get(self.count_key, 0) + 1
            with open(LOG, "a", encoding="utf-8") as f:
                f.write(f"{self.count_key} {self.path}\n")
        if self.mode == "slow":
            time.sleep(10)
        if self.mode == "redirect":
            self.send_response(302)
            self.send_header("Location", "http://127.0.0.1/final")
            self.end_headers()
            return
        if self.mode == "huge":
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(80 << 10))
            self.end_headers()
            self.wfile.write(b"x" * (80 << 10))
            return
        if self.mode == "error":
            self.send_response(503)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write(b"fixture error")
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", "2")
        self.end_headers()
        self.wfile.write(b"ok")


class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True


def main():
    servers = []
    for host, port, mode in FIXTURES:
        cls = type(f"Handler_{host.replace('.', '_')}_{mode}", (Handler,), {"mode": mode, "count_key": f"{host}:{port}"})
        srv = Server((host, port), cls)
        thread = threading.Thread(target=srv.serve_forever, daemon=True)
        thread.start()
        servers.append(srv)
        print(f"fixture {host}:{port} {mode}", flush=True)
    threading.Event().wait()


if __name__ == "__main__":
    main()
