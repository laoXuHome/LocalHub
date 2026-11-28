#!/usr/bin/env python3
"""P4.3 local fixture: first three requests slow (round 1 timeout), next requests OK (round 2)."""
import http.server, socketserver, threading, time
count = 0
lock = threading.Lock()
class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, fmt, *args): pass
    def do_GET(self):
        global count
        with lock:
            count += 1
            this = count
        print(f"request {this}", flush=True)
        if this <= 3:
            time.sleep(2.0)
        body=b'ok'
        self.send_response(200)
        self.send_header('Content-Type','text/plain')
        self.send_header('Content-Length',str(len(body)))
        self.end_headers()
        self.wfile.write(body)
class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address=True
    daemon_threads=True
with Server(('127.0.0.12',80), Handler) as s:
    s.serve_forever()
