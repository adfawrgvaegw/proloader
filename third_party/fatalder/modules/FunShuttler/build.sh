go build -v -ldflags "-s -w" -o FunShuttler_android_aarch64
GOOS=windows GOARCH=amd64 go build -v -ldflags "-s -w" -o FunShuttler_windows_amd64.exe
GOOS=linux GOARCH=amd64 go build -v -ldflags "-s -w" -o FunShuttler_linux_amd64
GOOS=linux GOARCH=arm64 go build -v -ldflags "-s -w" -o FunShuttler_linux_arm64
GOOS=windows GOARCH=arm64 go build -v -ldflags "-s -w" -o FunShuttler_windows_arm64.exe
