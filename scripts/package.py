"""Package clean release builds and collect notices from the local Go module cache."""
import argparse
import hashlib
from pathlib import Path
import re
import subprocess
import zipfile


def run(*args):
    return subprocess.check_output(args, text=True).strip()


def notices(binary):
    modules = Path(run("go", "env", "GOMODCACHE"))
    goroot = Path(run("go", "env", "GOROOT"))
    parts = [("Go", (goroot / "LICENSE").read_bytes())]
    for line in run("go", "version", "-m", binary).splitlines():
        fields = line.split()
        if fields[:1] != ["dep"]:
            continue
        module, version = fields[1:3]
        key = re.sub(r"[A-Z]", lambda m: "!" + m[0].lower(), module)
        root = modules / (key + "@" + version)
        licenses = sorted(p for p in root.iterdir() if p.is_file() and
                          p.name.upper().startswith(("LICENSE", "LICENCE", "COPYING", "NOTICE")))
        if not licenses:
            raise ValueError(f"Missing license for {module}")
        parts.extend((f"{module} {version} / {p.name}", p.read_bytes()) for p in licenses)
    return b"\n\n".join(title.encode() + b"\n" + data for title, data in parts)


def package(path, files, license_text):
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for src, name in files:
            archive.write(src, name)
        archive.writestr("THIRD-PARTY-NOTICES.txt", license_text)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for name in ["windows_exe", "mac_app", "output"]:
        parser.add_argument(name, type=Path)
    args = parser.parse_args()
    args.output.mkdir(exist_ok=True, parents=True)
    windows = args.output / "LANSend-Windows-x64.zip"
    mac = args.output / "LANSend-macOS-universal.zip"
    package(windows, [(args.windows_exe, "LANSend.exe")], notices(args.windows_exe))
    resources = "Contents/Resources/"
    workflow = resources + "发送到 Windows.workflow/Contents/"
    members = ["Contents/MacOS/LANSend", "Contents/Info.plist", resources + "engine",
               resources + "Setup.sh", resources + "AppIcon.icns", workflow + "Info.plist",
               workflow + "document.wflow", "Contents/_CodeSignature/CodeResources"]
    package(mac, [(args.mac_app / p, "LANSend.app/" + p) for p in members],
            notices(args.mac_app / resources / "engine"))
    (args.output / "SHA256SUMS.txt").write_text("".join(
        f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n" for path in [windows, mac]))
