#!/usr/bin/env python3
"""Tiny public health endpoint (port 8088): /health -> small JSON. Exposes no secrets, only role/uptime/disk (+ app status on main)."""
import json, os, shutil, socket, subprocess, time, urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer

ROLE = os.environ.get("FLEET_SELF", "unknown")

def probe(url):
    try:
        with urllib.request.urlopen(url, timeout=4) as r:
            return r.status
    except Exception:
        return None

def container(name):
    try:
        out = subprocess.run(["docker", "inspect", "-f", "{{.State.Status}}/{{if .State.Health}}{{.State.Health.Status}}{{else}}-{{end}}", name],
                             capture_output=True, text=True, timeout=5).stdout.strip()
        return out or "missing"
    except Exception:
        return "unknown"

class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        if self.path.split("?")[0] not in ("/health", "/"):
            self.send_response(404); self.end_headers(); return
        du = shutil.disk_usage("/")
        body = {"role": ROLE, "host": socket.gethostname(), "ok": True, "uptime_s": int(float(open("/proc/uptime").read().split()[0])),
                "disk_used_pct": round(du.used / du.total * 100, 1), "ts": int(time.time())}
        if ROLE == "api":
            body["backend"] = probe("http://127.0.0.1:8081/health")
            body["frontend"] = probe("http://127.0.0.1:3000/login")
            body["ok"] = body["backend"] == 200 and body["frontend"] == 200
        elif ROLE == "db":
            body["postgres"] = container("halal_equity_postgres")
            body["ok"] = body["postgres"].endswith("healthy") and body["postgres"].startswith("running")
        elif ROLE == "jobs":
            body["worker"] = container("halal_equity_worker")
            body["tunnel"] = container("halal_equity_postgres")
            body["ok"] = body["worker"].startswith("running") and body["tunnel"].startswith("running")
        data = json.dumps(body).encode()
        self.send_response(200 if body["ok"] else 503)
        self.send_header("content-type", "application/json"); self.send_header("cache-control", "no-store")
        self.send_header("content-length", str(len(data))); self.end_headers(); self.wfile.write(data)

HTTPServer(("0.0.0.0", 8088), H).serve_forever()
