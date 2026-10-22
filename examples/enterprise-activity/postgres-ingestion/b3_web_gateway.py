#!/usr/bin/env python3
"""Loopback-only authenticated-operator fixture for the existing web gateway.

This is a browser acceptance harness, not IAM or a deployable product gateway.
The independently running Core API still resolves its configured principal and
rechecks current rights. Untrusted client identity headers are always replaced.
"""
import http.client
import http.server
import threading

def start(port,upstream,principal,token):
    class Gateway(http.server.BaseHTTPRequestHandler):
        def log_message(self,*args): pass
        def do_GET(self): self.forward()
        def do_POST(self): self.forward()
        def forward(self):
            if self.client_address[0]!="127.0.0.1":self.send_error(403);return
            size=int(self.headers.get("Content-Length","0"))
            if size>2*1024*1024:self.send_error(413);return
            data=self.rfile.read(size) if size else None
            skip={"host","connection","x-delivery-web-gateway","x-authenticated-principal","transfer-encoding"}
            headers={k:v for k,v in self.headers.items() if k.lower() not in skip}
            # Preserve the public authority for Next Server Actions' Origin
            # check; changing it would invalidate legitimate CSRF protection.
            authority=self.headers.get("Host","")
            headers.update({"Host":authority,"X-Forwarded-Host":authority,"x-delivery-web-gateway":token,"x-authenticated-principal":principal})
            conn=http.client.HTTPConnection("127.0.0.1",upstream,timeout=90)
            try:
                conn.request(self.command,self.path,body=data,headers=headers)
                response=conn.getresponse();payload=response.read()
                self.send_response(response.status)
                for k,v in response.getheaders():
                    if k.lower() not in {"transfer-encoding","connection","content-length"}:self.send_header(k,v)
                self.send_header("Content-Length",str(len(payload)));self.end_headers();self.wfile.write(payload)
            except (OSError,http.client.HTTPException):self.send_error(502)
            finally:conn.close()
    server=http.server.ThreadingHTTPServer(("127.0.0.1",port),Gateway)
    threading.Thread(target=server.serve_forever,daemon=True).start()
    return server
