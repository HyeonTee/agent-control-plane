#!/usr/bin/env python3
"""Optional local credential bridge for agents without native OAuth storage.

Only the approval link and user code are printed during login. Long-lived
credentials stay in a mode-0600 file outside the repository and model output.
"""

import argparse
import contextlib
import fcntl
import json
import os
from pathlib import Path
import sys
import tempfile
import time
import webbrowser
from urllib.error import HTTPError
from urllib.parse import quote, urlencode, urlparse
from urllib.request import HTTPRedirectHandler, Request, build_opener


BASE_URL = os.environ.get("HUB_URL", "https://agent.gwinam.com").rstrip("/")
CLIENT_ID = "agent-control-plane"
CONFIG_DIR = Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config")) / "agent-control-plane"
CREDS = CONFIG_DIR / "credentials.json"
PENDING = CONFIG_DIR / "pending.json"
LOCK = CONFIG_DIR / ".lock"


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


OPENER = build_opener(NoRedirect)


def check_origin():
    parsed = urlparse(BASE_URL)
    if (parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password
            or parsed.path or parsed.query or parsed.fragment):
        raise ValueError("HUB_URL must be an HTTPS origin")


def prepare_dir():
    if CONFIG_DIR.is_symlink():
        raise ValueError("credential directory may not be a symlink")
    CONFIG_DIR.mkdir(mode=0o700, parents=True, exist_ok=True)
    info = CONFIG_DIR.stat()
    if info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError("credential directory must have mode 0700")


def read_private(path):
    info = path.stat()
    if path.is_symlink() or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError(f"unsafe credential file: {path}")
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def write_private(path, value):
    descriptor, temporary = tempfile.mkstemp(dir=CONFIG_DIR, prefix=".credential-")
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            json.dump(value, handle)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def form(path, values):
    return request("POST", path, urlencode(values).encode(), "application/x-www-form-urlencoded")


def request(method, path, body=None, content_type=None, token=None, key=None):
    if not path.startswith("/") or path.startswith("//") or ".." in path.split("?")[0].split("/"):
        raise ValueError("path must stay on the Hub origin")
    headers = {"Accept": "application/json"}
    if content_type:
        headers["Content-Type"] = content_type
    if token:
        headers["Authorization"] = "Bearer " + token
    if key:
        headers["Idempotency-Key"] = key
    call = Request(BASE_URL + path, data=body, headers=headers, method=method)
    try:
        with OPENER.open(call, timeout=15) as response:
            return response.status, response.read(1 << 20)
    except HTTPError as error:
        return error.code, error.read(1 << 20)


def locked(operation):
    prepare_dir()
    descriptor = os.open(LOCK, os.O_RDWR | os.O_CREAT | getattr(os, "O_NOFOLLOW", 0), 0o600)
    try:
        info = os.fstat(descriptor)
        if info.st_uid != os.getuid() or info.st_mode & 0o077:
            raise ValueError("unsafe credential lock")
        fcntl.flock(descriptor, fcntl.LOCK_EX)
        return operation()
    finally:
        fcntl.flock(descriptor, fcntl.LOCK_UN)
        os.close(descriptor)


def start(args):
    def run():
        status, data = form("/oauth/device_authorization", {
            "client_id": CLIENT_ID, "client_label": args.label, "scope": args.scope,
        })
        if status != 200:
            raise RuntimeError(f"device authorization failed ({status}): {data.decode()}")
        result = json.loads(data)
        if result.get("verification_uri") != BASE_URL + "/activate":
            raise RuntimeError("Hub returned an unexpected approval URL")
        write_private(PENDING, {
            "device_code": result["device_code"], "expires_at": time.time() + result["expires_in"],
            "interval": result["interval"], "origin": BASE_URL,
        })
        # Built locally so the link can only point at the checked Hub origin.
        link = BASE_URL + "/activate?code=" + quote(result["user_code"])
        print("Approve in your browser:", link)
        print("Confirm the page shows code:", result["user_code"])
        print("Then run: python3 scripts/hub-credentials.py finish")
        if not args.no_browser:
            with contextlib.suppress(webbrowser.Error):
                webbrowser.open(link)
    locked(run)


def finish(_args):
    def run():
        pending = read_private(PENDING)
        if pending.get("origin") != BASE_URL:
            raise ValueError("pending authorization belongs to another Hub origin")
        interval = max(5, int(pending["interval"]))
        while time.time() < pending["expires_at"]:
            status, data = form("/oauth/token", {
                "client_id": CLIENT_ID,
                "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
                "device_code": pending["device_code"],
            })
            result = json.loads(data)
            if status == 200:
                write_private(CREDS, {
                    "access_token": result["access_token"], "refresh_token": result["refresh_token"],
                    "access_expires_at": time.time() + result["expires_in"], "origin": BASE_URL,
                })
                PENDING.unlink()
                print("Hub connected. Credentials saved locally.")
                return
            if result.get("error") == "slow_down":
                interval += 5
            elif result.get("error") != "authorization_pending":
                raise RuntimeError("device authorization was denied or expired")
            time.sleep(interval)
        raise RuntimeError("device authorization expired")
    locked(run)


def refresh_credentials(credentials):
    status, data = form("/oauth/token", {
        "client_id": CLIENT_ID, "grant_type": "refresh_token",
        "refresh_token": credentials["refresh_token"],
    })
    if status != 200:
        raise RuntimeError("Hub login expired or revoked; reconnect with start and finish")
    result = json.loads(data)
    credentials = {
        "access_token": result["access_token"], "refresh_token": result["refresh_token"],
        "access_expires_at": time.time() + result["expires_in"], "origin": BASE_URL,
    }
    write_private(CREDS, credentials)
    return credentials


def api(args):
    if not args.path.startswith("/api/v1/"):
        raise ValueError("API request path must start with /api/v1/")
    body = args.json.encode() if args.json is not None else None
    def run():
        credentials = read_private(CREDS)
        if credentials.get("origin") != BASE_URL:
            raise ValueError("credentials belong to another Hub origin")
        if time.time() + 60 >= credentials["access_expires_at"]:
            credentials = refresh_credentials(credentials)
        status, data = request(args.method, args.path, body,
            "application/json" if body is not None else None,
            credentials["access_token"], args.idempotency_key)
        if status == 401:
            credentials = refresh_credentials(credentials)
            status, data = request(args.method, args.path, body,
                "application/json" if body is not None else None,
                credentials["access_token"], args.idempotency_key)
        sys.stdout.buffer.write(data)
        if data and not data.endswith(b"\n"):
            print()
        if status >= 400:
            raise RuntimeError(f"Hub returned HTTP {status}")
    locked(run)


def main():
    check_origin()
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    start_parser = commands.add_parser("start")
    start_parser.add_argument("--label", required=True)
    start_parser.add_argument("--scope", default="context:read")
    start_parser.add_argument("--no-browser", action="store_true", help="print the approval link without opening it")
    start_parser.set_defaults(action=start)
    commands.add_parser("finish").set_defaults(action=finish)
    api_parser = commands.add_parser("api")
    api_parser.add_argument("method", choices=["GET", "POST", "PATCH", "PUT", "DELETE"])
    api_parser.add_argument("path")
    api_parser.add_argument("--json")
    api_parser.add_argument("--idempotency-key")
    api_parser.set_defaults(action=api)
    args = parser.parse_args()
    try:
        args.action(args)
    except (ValueError, OSError, KeyError, RuntimeError, json.JSONDecodeError) as error:
        print(f"hub-credentials: {error}", file=sys.stderr)
        raise SystemExit(1)


if __name__ == "__main__":
    main()
