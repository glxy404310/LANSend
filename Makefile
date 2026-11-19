.PHONY: test integration windows macos

test:
	go test ./...

integration:
	go test -tags integration -run '^TestEmbeddedTransfer$$' -timeout 120s .

windows:
	mkdir -p build/windows-amd64 build/windows-arm64
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w -H=windowsgui' -o build/windows-amd64/LANSend.exe .
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags='-s -w -H=windowsgui' -o build/windows-arm64/LANSend.exe .

macos:
	./mac-desktop/build.sh
