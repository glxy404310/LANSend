#!/bin/bash
# Run on a disposable macOS runner in its logged-in desktop session.
set -euo pipefail
app="$(cd "$1" && pwd)"
engine="$app/Contents/Resources/engine"
test_home=$(mktemp -d)
export HOME="$test_home" CFFIXED_USER_HOME="$test_home"
codesign --verify --deep --strict "$app"
for binary in "$engine" "$app/Contents/MacOS/LANSend"; do lipo "$binary" -verify_arch arm64 x86_64; done
xcrun clang -fobjc-arc -framework Cocoa mac-desktop/tests/receipts.m -o "$test_home/receipt-test"
"$test_home/receipt-test"
"$app/Contents/MacOS/LANSend" --background-receiver &
app_pid=$!
trap 'kill "${engine_pid:-}" "$app_pid" 2>/dev/null || true' EXIT
config="$HOME/Library/Application Support/LANSend"
for i in {1..60}; do [[ -s "$config/identity.json" ]] && break; sleep .25; done
"$engine" ready
engine_pid=$(pgrep -P "$app_pid" -x engine)
test -n "$engine_pid"
python3 - "$config" <<'PY'
import hashlib,json,pathlib,ssl,sys
root=pathlib.Path(sys.argv[1])
cert=json.loads((root/'identity.json').read_text())['Certificate']
pin=hashlib.sha256(ssl.PEM_cert_to_DER_cert(cert)).hexdigest()
(root/'peer.json').write_text(json.dumps({'Address':'127.0.0.1:53318','PeerHash':pin}))
PY
printf 'Mac receipt QA' > "$HOME/收件.txt"
"$engine" send "$HOME/收件.txt" "$HOME/收件.txt"
cmp "$HOME/收件.txt" "$HOME/Downloads/LANSend/收件 (1).txt"
touch "$config/silent-receipts"
"$engine" send "$HOME/收件.txt"
cmp "$HOME/收件.txt" "$HOME/Downloads/LANSend/收件 (2).txt"
echo 'PASS Mac packaged app, direct engine, HTTPS batch/silent saves, native mode and window regression'
