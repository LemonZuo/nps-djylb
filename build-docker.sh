#!/bin/bash

set -e
set -o pipefail

# ========== 全局变量 ==========
OS_NAME=$(uname)
ARCH=$(uname -m)
SCRIPT_START_TIME=$(date +%s)
VERSION=""
HUB_USER=""
BUILDER_NAME="multi-platform-build"
MENU_SELECTION=""  # 用于存储菜单选择

# 时间记录关联数组
declare -A BUILD_TIMES
declare -A BUILD_STATUS

# ========== 工具函数 ==========

# 检查操作系统
check_os() {
    if [ "$OS_NAME" != "Darwin" ] && [ "$OS_NAME" != "Linux" ]; then
        echo "错误: 不支持的操作系统: $OS_NAME"
        exit 1
    fi
}

# 设置构建环境
setup_environment() {
    echo "========== 设置构建环境 =========="
    
    # 设置构建优化环境变量
    export DOCKER_BUILDKIT=1
    export BUILDKIT_PROGRESS=plain
    export QEMU_CPU=max
    
    # 检测架构并设置 QEMU
    if [ "$ARCH" != "arm64" ]; then
        echo "正在设置 QEMU 用于多架构构建..."
        docker run --rm --privileged multiarch/qemu-user-static --reset -p yes
    else
        echo "运行在 ARM64 架构，跳过 QEMU 设置"
    fi
}

# Docker 登录（只执行一次）
docker_login_once() {
    echo "========== Docker Hub 登录 =========="
    
    # 从 .env 文件导入环境变量
    if [ -f ".env" ]; then
        export $(cat .env | sed 's/#.*//g' | xargs)
    else
        echo "错误: .env 文件未找到"
        exit 1
    fi
    
    # 检查是否已登录
    echo "检查 Docker Hub 登录状态..."
    docker_login_check=$(docker info 2>/dev/null | grep -i "username:" || echo "not_logged_in")
    
    if echo "$docker_login_check" | grep -q "not_logged_in"; then
        echo "未登录 Docker Hub，正在尝试登录..."
        docker login -u="${HUB_USER}" -p="${HUB_PASS}"
        status=$?
        
        if [ $status -ne 0 ]; then
            echo "错误: Docker 登录失败"
            exit $status
        else
            echo "Docker 登录成功"
        fi
    else
        echo "已登录 Docker Hub"
    fi
}

# 设置 buildx（只执行一次）
setup_buildx_once() {
    echo "========== 设置 Docker Buildx =========="
    
    # 创建并使用 buildx 构建器
    docker buildx create \
      --name $BUILDER_NAME \
      --driver docker-container \
      --driver-opt env.BUILDKIT_STEP_LOG_MAX_SIZE=10485760 \
      --driver-opt env.BUILDKIT_STEP_LOG_MAX_SPEED=10485760 \
      --use || true
    
    docker buildx use $BUILDER_NAME
    docker buildx inspect --bootstrap
}

# 获取版本号（只执行一次）
get_version_once() {
    echo "========== 获取版本号 =========="
    
    VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "0.0.1")
    # 去除版本号开头的 'v'
    VERSION=$(echo $VERSION | sed 's/^v//')
    echo "使用版本: $VERSION"
}

# 格式化时间显示
format_time() {
    local timestamp=$1
    
    if [ "$OS_NAME" = "Darwin" ]; then
        date -r $timestamp '+%Y-%m-%d %H:%M:%S'
    elif [ "$OS_NAME" = "Linux" ]; then
        date -d @$timestamp '+%Y-%m-%d %H:%M:%S'
    fi
}

# 构建单个镜像
build_image() {
    local image_type=$1
    local start_time=$(date +%s)
    
    echo ""
    echo "========== 构建 ${image_type} 镜像 =========="
    
    # 动态设置仓库名
    local repo_name="${HUB_USER}/djylb-${image_type}"
    local dockerfile="${image_type}.Dockerfile"
    
    # 检查 Dockerfile 是否存在
    if [ ! -f "$dockerfile" ]; then
        echo "错误: ${dockerfile} 文件不存在"
        BUILD_STATUS[${image_type}]="失败 - Dockerfile 不存在"
        return 1
    fi
    
    echo "镜像仓库: ${repo_name}"
    echo "Dockerfile: ${dockerfile}"
    echo "版本标签: ${VERSION}"
    echo "开始构建..."
    
    # 执行构建
    if docker buildx build \
        --cache-from=type=registry,ref=${repo_name}:cache \
        --cache-to=type=registry,ref=${repo_name}:cache,mode=max \
        --platform linux/amd64,linux/arm,linux/arm64 \
        --build-arg JOBS=4 \
        --build-arg MAKEFLAGS="-j4" \
        --build-arg BUILDKIT_INLINE_CACHE=1 \
        -f ${dockerfile} \
        -t ${repo_name}:${VERSION} \
        -t ${repo_name}:latest . \
        --push \
        --progress=plain; then
        
        local end_time=$(date +%s)
        local elapsed=$((end_time - start_time))
        BUILD_TIMES[${image_type}]=$elapsed
        BUILD_STATUS[${image_type}]="成功"
        echo "✅ ${image_type} 构建成功，耗时: ${elapsed} 秒"
    else
        BUILD_STATUS[${image_type}]="失败"
        echo "❌ ${image_type} 构建失败"
        return 1
    fi
}

# 打印构建汇总
print_summary() {
    local script_end_time=$(date +%s)
    local total_elapsed=$((script_end_time - SCRIPT_START_TIME))
    
    echo ""
    echo "=========================================="
    echo "              构建汇总报告                "
    echo "=========================================="
    
    # 打印各镜像构建状态和时间
    echo ""
    echo "镜像构建详情:"
    echo "------------------------------------------"
    
    for image_type in "${!BUILD_STATUS[@]}"; do
        local status="${BUILD_STATUS[${image_type}]}"
        local time_str=""
        
        if [[ -n "${BUILD_TIMES[${image_type}]}" ]]; then
            time_str=" (${BUILD_TIMES[${image_type}]} 秒)"
        fi
        
        printf "  %-10s : %s%s\n" "${image_type}" "${status}" "${time_str}"
    done
    
    # 打印时间统计
    echo ""
    echo "时间统计:"
    echo "------------------------------------------"
    echo "  开始时间: $(format_time $SCRIPT_START_TIME)"
    echo "  结束时间: $(format_time $script_end_time)"
    echo "  总耗时:   ${total_elapsed} 秒"
    
    # 打印成功推送的镜像
    echo ""
    echo "成功推送的镜像:"
    echo "------------------------------------------"
    
    for image_type in "${!BUILD_STATUS[@]}"; do
        if [[ "${BUILD_STATUS[${image_type}]}" == "成功" ]]; then
            echo "  ✅ ${HUB_USER}/${image_type}:${VERSION}"
            echo "  ✅ ${HUB_USER}/${image_type}:latest"
        fi
    done
    
    echo "=========================================="
}

# 显示使用说明
show_usage() {
    echo "使用方法: $0 [nps|npc|all]"
    echo ""
    echo "参数说明:"
    echo "  nps  - 只构建 nps 镜像"
    echo "  npc  - 只构建 npc 镜像"
    echo "  all  - 构建所有镜像（默认）"
    echo ""
    echo "示例:"
    echo "  $0        # 显示交互式菜单"
    echo "  $0 nps    # 只构建 nps"
    echo "  $0 npc    # 只构建 npc"
    echo "  $0 all    # 构建所有镜像"
}

# 显示交互式菜单
show_menu() {
    echo ""
    echo "=========================================="
    echo "        Docker 镜像构建脚本 v2.0         "
    echo "=========================================="
    echo ""
    echo "请选择要构建的镜像:"
    echo ""
    echo "  1) 构建 NPS 镜像"
    echo "  2) 构建 NPC 镜像"
    echo "  3) 构建所有镜像 (NPS + NPC)"
    echo "  4) 退出"
    echo ""
    echo "------------------------------------------"
    read -p "请输入选项 [1-4]: " choice
    echo ""
    
    case $choice in
        1)
            echo "您选择了: 构建 NPS 镜像"
            MENU_SELECTION="nps"
            ;;
        2)
            echo "您选择了: 构建 NPC 镜像"
            MENU_SELECTION="npc"
            ;;
        3)
            echo "您选择了: 构建所有镜像"
            MENU_SELECTION="all"
            ;;
        4)
            echo "退出脚本..."
            exit 0
            ;;
        *)
            echo "错误: 无效的选项 '$choice'"
            echo "请输入 1-4 之间的数字"
            sleep 2
            show_menu
            ;;
    esac
}

# ========== 主函数 ==========
main() {
    local build_target=""
    
    # 如果没有提供参数，显示交互式菜单
    if [ $# -eq 0 ]; then
        show_menu
        build_target="$MENU_SELECTION"
    else
        # 有参数时，使用命令行参数
        build_target="$1"
        
        # 参数验证
        case "$build_target" in
            nps|npc|all)
                echo "=========================================="
                echo "        Docker 镜像构建脚本 v2.0         "
                echo "=========================================="
                echo "构建目标: ${build_target}"
                ;;
            -h|--help|help)
                show_usage
                exit 0
                ;;
            *)
                echo "错误: 无效的构建目标 '${build_target}'"
                echo ""
                show_usage
                exit 1
                ;;
        esac
    fi
    
    echo ""
    
    # 执行一次性初始化操作
    check_os
    setup_environment
    docker_login_once
    setup_buildx_once
    get_version_once
    
    # 根据参数执行构建
    case "$build_target" in
        nps)
            build_image "nps"
            ;;
        npc)
            build_image "npc"
            ;;
        all)
            build_image "nps"
            build_image "npc"
            ;;
    esac
    
    # 打印汇总报告
    print_summary
}

# ========== 脚本入口 ==========
# 设置退出时的清理操作
trap 'echo ""; echo "构建脚本执行完毕"' EXIT

# 执行主函数
main "$@"