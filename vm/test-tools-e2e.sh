#!/usr/bin/env bash
set -euo pipefail

VM_HOST="${VM_HOST:-192.168.64.5}"
VM_USER="${VM_USER:-oscarcode91}"
VM_PASS="${VM_PASS:-12345}"
MOCK_PORT="${MOCK_PORT:-5599}"
GATEWAY_PORT="${GATEWAY_PORT:-4391}"

sshpass -p "$VM_PASS" ssh -o StrictHostKeyChecking=no "$VM_USER@$VM_HOST" 'bash -s' <<REMOTE
set -euo pipefail

tmproot="\$(mktemp -d /tmp/claw-tools-e2e.XXXXXX)"
mock_port="$MOCK_PORT"
gateway_port="$GATEWAY_PORT"
user_name="\$(id -un)"
real_home="\$HOME"
uid_num="\$(id -u)"
export DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/\$uid_num/bus"
export XDG_RUNTIME_DIR="/run/user/\$uid_num"
export REAL_HOME="\$real_home"
export E2E_USER="\$user_name"

cleanup() {
  kill "\${GW_PID:-}" "\${MOCK_PID:-}" 2>/dev/null || true
  rm -rf "\$tmproot"
}
trap cleanup EXIT

mkdir -p "\$tmproot/home"
mkdir -p "\$tmproot/home/.openclaw/agents/main/agent"
mkdir -p "\$tmproot/home/.openclaw/state/sessions"
mkdir -p "\$tmproot/home/.openclaw/workspace/skills"
mkdir -p "\$tmproot/home/.openclaw/skills/managed"

cat > "\$tmproot/home/.openclaw/openclaw.json" <<JSON
{
  "agent": {
    "model": "github-copilot/gpt-5.4",
    "workspace": "\$tmproot/home/.openclaw/workspace",
    "provider": "copilot-proxy",
    "baseUrl": "http://127.0.0.1:\$mock_port/openai/v1"
  },
  "setup": {
    "providerPending": false,
    "bootstrapReady": true
  }
}
JSON

cat > "\$tmproot/home/.openclaw/agents/main/agent/auth-profiles.json" <<JSON
{
  "version": 1,
  "profiles": {
    "copilot-proxy:default": {
      "provider": "copilot-proxy",
      "mode": "token",
      "token": ""
    }
  }
}
JSON

cat > "\$tmproot/home/.openclaw/skills/managed/.anthropic-default-skills.json" <<JSON
{
  "version": 1,
  "source": "https://github.com/anthropics/skills/tree/main/skills",
  "installedAt": "2026-04-29T00:00:00Z",
  "installed": [],
  "skipped": []
}
JSON

cat > "\$tmproot/home/.openclaw/workspace/AGENTS.md" <<'EOF2'
# AGENTS

This workspace is managed by elementary-claw.
EOF2

cat > "\$tmproot/home/.openclaw/workspace/IDENTITY.md" <<'EOF2'
# IDENTITY

- assistant_name: Samantha
- assistant_nature: A local AI teammate for this computer
- assistant_vibe: direct, warm, and pragmatic
EOF2

cat > "\$tmproot/home/.openclaw/workspace/SOUL.md" <<'EOF2'
# SOUL

Samantha should be helpful without filler, pragmatic, and respectful of user intent.
EOF2

cat > "\$tmproot/home/.openclaw/workspace/USER.md" <<EOF2
# USER

- account_name: \$user_name
- preferred_name: \$user_name
EOF2

cat > "\$tmproot/home/.openclaw/workspace/TOOLS.md" <<'EOF2'
# TOOLS

- filesystem
- process
- notifications
EOF2

cat > "\$tmproot/home/.openclaw/workspace/HEARTBEAT.md" <<'EOF2'
# HEARTBEAT

Resume prior context, protect user data, and keep the local machine stable.
EOF2

cat > "\$tmproot/mock_openai.py" <<'PY'
#!/usr/bin/env python3
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

REAL_HOME = os.environ.get("REAL_HOME", "/tmp")
TOOL_MAP = {
    "E2E:get_battery_status": ("get_battery_status", {}),
    "E2E:send_notification": (
        "send_notification",
        {
            "summary": "Codex E2E",
            "body": "VM end-to-end notification",
            "expire_seconds": 2,
        },
    ),
    "E2E:get_network_status": ("get_network_status", {}),
    "E2E:open_folder": ("open_folder", {"path": os.path.join(REAL_HOME, "Downloads")}),
    "E2E:get_current_user": ("get_current_user", {}),
    "E2E:take_screenshot": ("take_screenshot", {"filename": "codex-e2e-shot.png"}),
}

class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        return

    def _write_json(self, status, payload):
        data = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        if self.path != "/openai/v1/chat/completions":
            self._write_json(404, {"error": "not found"})
            return

        length = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(length) or b"{}")
        messages = payload.get("messages") or []

        tool_messages = [m for m in messages if m.get("role") == "tool"]
        if tool_messages:
            tool_content = tool_messages[-1].get("content", "")
            response = {
                "id": "chatcmpl-final",
                "object": "chat.completion",
                "choices": [{
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": tool_content,
                    },
                    "finish_reason": "stop",
                }],
            }
            self._write_json(200, response)
            return

        user_content = ""
        for msg in reversed(messages):
            if msg.get("role") == "user":
                user_content = str(msg.get("content", "")).strip()
                break

        if user_content not in TOOL_MAP:
            self._write_json(400, {"error": f"unknown e2e prompt: {user_content}"})
            return

        tool_name, args = TOOL_MAP[user_content]
        response = {
            "id": "chatcmpl-tool",
            "object": "chat.completion",
            "choices": [{
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": None,
                    "tool_calls": [{
                        "id": f"call_{tool_name}",
                        "type": "function",
                        "function": {
                            "name": tool_name,
                            "arguments": json.dumps(args, separators=(",", ":")),
                        },
                    }],
                },
                "finish_reason": "tool_calls",
            }],
        }
        self._write_json(200, response)

if __name__ == "__main__":
    port = int(sys.argv[1])
    HTTPServer(("127.0.0.1", port), Handler).serve_forever()
PY
chmod +x "\$tmproot/mock_openai.py"
python3 "\$tmproot/mock_openai.py" "\$mock_port" >"\$tmproot/mock.log" 2>&1 &
MOCK_PID=\$!

python3 - "\$mock_port" <<'PY'
import socket, sys, time
port = int(sys.argv[1])
for _ in range(100):
    with socket.socket() as s:
        s.settimeout(0.2)
        try:
            s.connect(("127.0.0.1", port))
            sys.exit(0)
        except OSError:
            time.sleep(0.1)
print("mock upstream did not start", file=sys.stderr)
sys.exit(1)
PY

HOME="\$tmproot/home" USER="\$user_name" LOGNAME="\$user_name" \
DBUS_SESSION_BUS_ADDRESS="\$DBUS_SESSION_BUS_ADDRESS" \
XDG_RUNTIME_DIR="\$XDG_RUNTIME_DIR" \
/usr/local/bin/claw gateway serve --listen "127.0.0.1:\$gateway_port" --workdir "\$real_home" >"\$tmproot/gateway.log" 2>&1 &
GW_PID=\$!

python3 - "\$gateway_port" <<'PY'
import socket, sys, time
port = int(sys.argv[1])
for _ in range(200):
    with socket.socket() as s:
        s.settimeout(0.2)
        try:
            s.connect(("127.0.0.1", port))
            sys.exit(0)
        except OSError:
            time.sleep(0.1)
print("gateway did not start", file=sys.stderr)
sys.exit(1)
PY

python3 - "\$gateway_port" "\$real_home" "\$tmproot/home" "\$user_name" <<'PY'
import json
import os
import pathlib
import sys
import urllib.request

port = int(sys.argv[1])
real_home = sys.argv[2]
temp_home = sys.argv[3]
user_name = sys.argv[4]
base = f"http://127.0.0.1:{port}"

cases = [
    ("get_battery_status", "E2E:get_battery_status"),
    ("send_notification", "E2E:send_notification"),
    ("get_network_status", "E2E:get_network_status"),
    ("open_folder", "E2E:open_folder"),
    ("get_current_user", "E2E:get_current_user"),
    ("take_screenshot", "E2E:take_screenshot"),
]

def post_json(url, payload):
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.loads(resp.read().decode("utf-8")), dict(resp.headers)

def get_json(url):
    with urllib.request.urlopen(url, timeout=30) as resp:
        return json.loads(resp.read().decode("utf-8"))

def assert_true(condition, message):
    if not condition:
        raise AssertionError(message)

summaries = []
health = get_json(base + "/healthz")
assert_true(health.get("ok") is True, f"gateway health failed: {health}")

for tool_name, prompt in cases:
    session_id = f"e2e-{tool_name}"
    response, headers = post_json(base + "/v1/chat/completions", {
        "model": "gpt-4o",
        "session_id": session_id,
        "messages": [{"role": "user", "content": prompt}],
    })

    assistant_content = response["choices"][0]["message"]["content"]
    tool_payload = json.loads(assistant_content)
    session = get_json(base + f"/v1/sessions/{session_id}")
    roles = [m["role"] for m in session["messages"]]

    assert_true(roles == ["user", "assistant", "tool", "assistant"], f"{tool_name}: unexpected session roles {roles}")
    called_name = session["messages"][1]["tool_calls"][0]["function"]["name"]
    assert_true(called_name == tool_name, f"{tool_name}: gateway called {called_name}")
    assert_true(session["messages"][2]["tool_call_id"] == f"call_{tool_name}", f"{tool_name}: wrong tool_call_id")

    if tool_name == "get_battery_status":
        assert_true("present" in tool_payload, f"{tool_name}: missing present field")
    elif tool_name == "send_notification":
        assert_true(tool_payload.get("ok") is True, f"{tool_name}: expected ok=true")
        assert_true(tool_payload.get("summary") == "Codex E2E", f"{tool_name}: wrong summary")
    elif tool_name == "get_network_status":
        assert_true("state" in tool_payload and "connected" in tool_payload, f"{tool_name}: incomplete payload {tool_payload}")
    elif tool_name == "open_folder":
        assert_true(tool_payload.get("ok") is True, f"{tool_name}: expected ok=true")
        assert_true(tool_payload.get("uri") == f"file://{real_home}/Downloads", f"{tool_name}: wrong uri {tool_payload.get('uri')}")
    elif tool_name == "get_current_user":
        assert_true(tool_payload.get("username") == user_name, f"{tool_name}: wrong username {tool_payload}")
    elif tool_name == "take_screenshot":
        path = tool_payload.get("path", "")
        assert_true(path.startswith(temp_home + "/Pictures/"), f"{tool_name}: wrong screenshot path {path}")
        assert_true(pathlib.Path(path).exists(), f"{tool_name}: screenshot file missing at {path}")
        pathlib.Path(path).unlink()

    summaries.append({
        "tool": tool_name,
        "session_id": session_id,
        "result": tool_payload,
        "x_session_id": headers.get("X-Session-Id", headers.get("X-Session-ID", "")),
    })

print(json.dumps({
    "ok": True,
    "gateway": base,
    "health": health,
    "results": summaries,
}, indent=2, ensure_ascii=False))
PY
REMOTE
