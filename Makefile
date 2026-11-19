.PHONY: windows macos package clean

windows:
	mkdir -p build/windows-amd64 build/windows-arm64
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w -H=windowsgui' -o build/windows-amd64/LANSend.exe .
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags='-s -w -H=windowsgui' -o build/windows-arm64/LANSend.exe .

macos:
	./mac-desktop/build.sh

package:
	python3 scripts/package.py build/windows-amd64/LANSend.exe build/LANSend.app dist

clean:
	rm -rf build dist
