#!/bin/bash
gvm use go1.24.5
# Linux
go generate github.com/Yeah114/Fatalder/constants && GOOS=linux GOARCH=amd64 garble -literals -tiny -seed=random build -ldflags "-s -w" -o Fatalder-cli_linux_amd64 -v github.com/Yeah114/Fatalder/cmd/cli && upx Fatalder-cli_linux_amd64 &
# Windows
go generate github.com/Yeah114/Fatalder/constants && GOOS=windows GOARCH=amd64 garble -literals -tiny -seed=random build -ldflags "-s -w" -o Fatalder-cli_windows_amd64.exe -v github.com/Yeah114/Fatalder/cmd/cli && upx Fatalder-cli_windows_amd64.exe &
# Android
go generate github.com/Yeah114/Fatalder/constants && GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC=/usr/lib/android-ndk/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android34-clang garble -literals -tiny -seed=random build -ldflags "-s -w" -o Fatalder-cli_android_aarch64 -v github.com/Yeah114/Fatalder/cmd/cli && upx Fatalder-cli