"""Local OTLP/HTTP stub for action tests that run without Dash0 secrets.

Fork pull requests get no secrets, so jobs that only test installation and
profile handling send to this stub instead of Dash0. It answers every POST
with an empty protobuf response, which OTLP clients read as success.
"""
import http.server
import sys


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        self.send_response(200)
        self.send_header("Content-Type", "application/x-protobuf")
        self.send_header("Content-Length", "0")
        self.end_headers()
        print(f"stub: {self.path}", flush=True)


http.server.HTTPServer(("127.0.0.1", int(sys.argv[1])), Handler).serve_forever()
