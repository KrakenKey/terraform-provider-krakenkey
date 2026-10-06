#!/usr/bin/env python3
"""Mock of the KrakenKey certificate API, for the option B prototype only.

Implements the endpoints krakenkey_certificate calls, with the real API's
field names, and signs CSRs with a throwaway two-level CA via the openssl CLI.
POST /_mock/renew/<id> simulates a server-side auto-renewal: same CSR, new
certificate, renewalCount + 1. Every request body is appended to requests.log
so the run can check that no private key was ever sent.

Usage: mockapi.py <workdir> <port>
"""
import datetime
import json
import os
import re
import subprocess
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

WORK, PORT = sys.argv[1], int(sys.argv[2])
API_KEY = "kk_mock_prototype"
CERTS = {}
NEXT_ID = [100]


def sh(*args, stdin=None):
    return subprocess.run(args, input=stdin, capture_output=True, text=True, check=True).stdout


def setup_ca():
    os.makedirs(WORK, exist_ok=True)
    root, inter = f"{WORK}/root", f"{WORK}/inter"
    if os.path.exists(f"{inter}.pem"):
        return
    sh("openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
       "-keyout", f"{root}.key", "-out", f"{root}.pem", "-days", "3650", "-subj", "/CN=Mock Root")
    sh("openssl", "req", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
       "-keyout", f"{inter}.key", "-out", f"{inter}.csr", "-subj", "/CN=Mock Intermediate")
    with open(f"{WORK}/ca.ext", "w") as f:
        f.write("basicConstraints=critical,CA:true,pathlen:0\nkeyUsage=critical,keyCertSign,cRLSign\n")
    sh("openssl", "x509", "-req", "-in", f"{inter}.csr", "-CA", f"{root}.pem", "-CAkey", f"{root}.key",
       "-CAcreateserial", "-out", f"{inter}.pem", "-days", "1825", "-extfile", f"{WORK}/ca.ext")


def sign(cert):
    """Sign the stored CSR. Each renewal gets one more day so notAfter changes."""
    csr = f"{WORK}/{cert['id']}.csr"
    with open(csr, "w") as f:
        f.write(cert["rawCsr"])
    names = re.findall(r"DNS:([^,\s]+)", sh("openssl", "req", "-in", csr, "-noout", "-text"))
    ext = f"{WORK}/{cert['id']}.ext"
    with open(ext, "w") as f:
        f.write("basicConstraints=CA:false\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\n")
        f.write("subjectAltName=" + ",".join("DNS:" + n for n in names) + "\n")
    days = 90 + cert["renewalCount"]
    crt = sh("openssl", "x509", "-req", "-in", csr, "-CA", f"{WORK}/inter.pem", "-CAkey", f"{WORK}/inter.key",
             "-CAcreateserial", "-days", str(days), "-extfile", ext)
    end = sh("openssl", "x509", "-noout", "-enddate", stdin=crt).strip().split("=", 1)[1]
    expires = datetime.datetime.strptime(end, "%b %d %H:%M:%S %Y %Z").replace(tzinfo=datetime.timezone.utc)
    cert.update(status="issued", crtPem=crt, chainPem=open(f"{WORK}/inter.pem").read(),
                expiresAt=expires.isoformat().replace("+00:00", ".000Z"))


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def send(self, code, body=None):
        data = json.dumps(body if body is not None else {}).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def handle_any(self, method):
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length).decode() if length else ""
        with open(f"{WORK}/requests.log", "a") as f:
            f.write(json.dumps({"method": method, "path": self.path, "body": raw}) + "\n")

        m = re.fullmatch(r"/_mock/renew/(\d+)", self.path)
        if m and method == "POST":
            cert = CERTS[int(m.group(1))]
            cert["renewalCount"] += 1
            sign(cert)
            return self.send(200, cert)

        if self.headers.get("Authorization") != f"Bearer {API_KEY}":
            return self.send(401, {"statusCode": 401, "message": "Unauthorized"})
        body = json.loads(raw) if raw else {}

        if self.path == "/certs/tls" and method == "POST":
            csr = body.get("csrPem", "")
            if "BEGIN CERTIFICATE REQUEST" not in csr:
                return self.send(400, {"statusCode": 400, "message": "Invalid CSR PEM format"})
            for c in CERTS.values():  # the real API returns the original for a repeat within 15 minutes
                if c["rawCsr"].strip() == csr.strip():
                    return self.send(201, {"id": c["id"], "status": c["status"]})
            cid = NEXT_ID[0]
            NEXT_ID[0] += 1
            CERTS[cid] = {"id": cid, "status": "pending", "rawCsr": csr, "crtPem": None, "chainPem": None,
                          "expiresAt": None, "autoRenew": True, "renewalCount": 0, "failureReason": None}
            return self.send(201, {"id": cid, "status": "pending"})

        m = re.fullmatch(r"/certs/tls/(\d+)(/chain|/revoke)?", self.path)
        if not m or int(m.group(1)) not in CERTS:
            return self.send(404, {"statusCode": 404, "message": "Not found"})
        cert, sub = CERTS[int(m.group(1))], m.group(2)
        if method == "GET" and sub is None:
            if cert["status"] == "pending":
                cert["status"] = "issuing"  # one poll in between, like the real queue
            elif cert["status"] == "issuing":
                sign(cert)
            return self.send(200, cert)
        if method == "GET" and sub == "/chain":
            if cert["status"] != "issued":
                return self.send(400, {"statusCode": 400, "message": "Certificate is not issued"})
            return self.send(200, {"chain": [], "chainPem": cert["chainPem"],
                                   "fullChainPem": cert["crtPem"] + cert["chainPem"]})
        if method == "PATCH" and sub is None:
            cert["autoRenew"] = bool(body.get("autoRenew", cert["autoRenew"]))
            return self.send(200, cert)
        if method == "POST" and sub == "/revoke":
            cert["status"] = "revoked"
            return self.send(200, {"id": cert["id"], "status": "revoking"})
        return self.send(405, {"statusCode": 405, "message": "Method not allowed"})

    def do_GET(self):
        self.handle_any("GET")

    def do_POST(self):
        self.handle_any("POST")

    def do_PATCH(self):
        self.handle_any("PATCH")


if __name__ == "__main__":
    setup_ca()
    ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
