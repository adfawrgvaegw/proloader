#!/bin/bash
# -seed=random
set -euo pipefail

# 配置
GH_PAT="${GH_PAT:-${GH_TOKEN:-}}"
export GH_TOKEN=$GH_PAT
ANDROID_NDK_ROOT="$ANDROID_HOME/ndk/27.0.12077973"
TAG="commit-$(git rev-parse HEAD)"
RELEASE_REPO="Yeah114/Fatalder-Release"

# 准备构建目录 
mkdir -p build

# 生成 constants
echo "Generating constants..."
go generate ./constants

# 创建发布（如果不存在）
echo "Checking release $TAG..."
if ! gh release view "$TAG" --repo "$RELEASE_REPO" &> /dev/null; then
    echo "Creating release $TAG..."
    commit_msg=$(git log -1 --pretty=%B)
    {
        printf 'Automated release from commit %s\n' "$(git rev-parse HEAD)"
        echo
        echo "Commit message:"
        printf '%s\n' "$commit_msg"
    } > release_notes.txt
    
    gh release create "$TAG" \
      --repo "$RELEASE_REPO" \
      --title "Fatalder CLI $TAG" \
      --notes-file release_notes.txt
    
    rm release_notes.txt
else
    echo "Release $TAG already exists."
fi

# 构建函数
build_and_upload() {
    local os=$1
    local arch=$2
    local ext=$3
    local cgo_enabled=${4:-0}
    local cc=${5:-}
    
    local output="build/Fatalder-cli_${os}_${arch}${ext}"
    
    # 检查文件是否已存在
    if [ -f "$output" ] && [ -f "${output}.sha" ]; then
        echo "Skipping ${os}-${arch}: $output already exists."
        return
    fi

    echo "Building ${os}-${arch}..."
    
    # 设置环境变量
    local env_vars=("GOOS=${os}" "GOARCH=${arch}" "CGO_ENABLED=${cgo_enabled}")
    if [ -n "$cc" ]; then
        env_vars+=("CC=${cc}")
    fi
    
    # 构建命令
    export "${env_vars[@]}"
    garble -seed=Yeah1145141 -literals -tiny build \
      -ldflags "-s -w -checklinkname=0" \
      -o "$output" \
      -v github.com/Yeah114/Fatalder/cmd/cli
    
    # 压缩
    echo "Compressing ${output}..."
    upx -fq "$output" || true
    
    # 计算 SHA512
    echo "Computing SHA512 for ${output}..."
    sha512sum "$output" | awk '{print $1}' > "${output}.sha"
    
    # 上传
    echo "Uploading ${output}..."
    gh release upload "$TAG" "$output" --repo "$RELEASE_REPO" --clobber
    gh release upload "$TAG" "${output}.sha" --repo "$RELEASE_REPO" --clobber
}

# 构建所有平台
ANDROID_CC="$ANDROID_NDK_ROOT/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android34-clang"
build_and_upload "android" "arm64" "" 1 "$ANDROID_CC"
build_and_upload "linux" "amd64" ""
build_and_upload "windows" "amd64" ".exe"
build_and_upload "darwin" "amd64" ""
build_and_upload "linux" "arm64" ""
build_and_upload "windows" "arm64" ".exe"
build_and_upload "darwin" "arm64" ""

# 构建 Android gRPC
export grpc_output="build/Fatalder-grpc_android_arm64"

# 检查 gRPC 文件是否已存在
if [ -f "$grpc_output" ] && [ -f "${grpc_output}.sha" ]; then
    echo "Skipping Android gRPC: $grpc_output already exists."
else
    echo "Building Android arm64 gRPC..."
    export GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC="$ANDROID_CC"
    garble -seed=Yeah1145141 -literals -tiny build \
      -ldflags "-s -w -checklinkname=0" \
      -o "$grpc_output" \
      -v github.com/Yeah114/Fatalder/cmd/grpc

    # 压缩
    echo "Compressing ${grpc_output}..."
    upx -fq "$grpc_output" || true

    # 计算 gRPC SHA512
    sha512sum "$grpc_output" | awk '{print $1}' > "${grpc_output}.sha"

    # 上传 gRPC 文件
    echo "Uploading gRPC files..."
    gh release upload "$TAG" "$grpc_output" --repo "$RELEASE_REPO" --clobber
    gh release upload "$TAG" "${grpc_output}.sha" --repo "$RELEASE_REPO" --clobber
fi

echo "All builds and releases completed successfully!"
