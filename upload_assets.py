#!/usr/bin/env python3
"""
素材库上传脚本（支持阿里云 OSS）
用法:
  python upload_assets.py <文件路径> --oss [--name <名称>] [--type <类型>]

环境变量配置（推荐在 .env 文件中配置）:
  OSS_ENDPOINT        OSS 地域节点，如 oss-cn-hangzhou.aliyuncs.com
  OSS_BUCKET           OSS Bucket 名称
  OSS_ACCESS_KEY_ID    AccessKey ID
  OSS_ACCESS_KEY_SECRET AccessKey Secret
  OSS_CUSTOM_DOMAIN    自定义域名（可选），如 cdn.example.com
  ASSETS_TOKEN         素材库 API token
  ASSETS_BASE_URL      素材库 API 地址，默认 https://ai.kkidc.com

示例:
  # 使用 OSS 上传
  python upload_assets.py image.png --oss

  # OSS + 指定名称和类型
  python upload_assets.py video.mp4 --oss --name "演示视频" --type Video

  # 临时托管（不加 --oss）
  python upload_assets.py image.png
"""

import argparse
import hashlib
import json
import mimetypes
import os
import sys
import time
from pathlib import Path

import requests

# ========== 从环境变量读取配置 ==========
BASE_URL = os.getenv("ASSETS_BASE_URL", "https://ai.kkidc.com")
TOKEN = os.getenv("ASSETS_TOKEN", "")
ASSETS_UPLOAD_URL = f"{BASE_URL}/api/assets/upload"

# OSS 配置
OSS_ENDPOINT = os.getenv("OSS_ENDPOINT", "")
OSS_BUCKET = os.getenv("OSS_BUCKET", "")
OSS_ACCESS_KEY_ID = os.getenv("OSS_ACCESS_KEY_ID", "")
OSS_ACCESS_KEY_SECRET = os.getenv("OSS_ACCESS_KEY_SECRET", "")
OSS_CUSTOM_DOMAIN = os.getenv("OSS_CUSTOM_DOMAIN", "")


# ================================================================
#                           工具函数
# ================================================================

def load_dotenv():
    """加载当前目录的 .env 文件"""
    env_file = Path.cwd() / ".env"
    if env_file.exists():
        with open(env_file, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                if "=" in line:
                    key, _, value = line.partition("=")
                    key = key.strip()
                    value = value.strip().strip('"').strip("'")
                    if key and key not in os.environ:
                        os.environ[key] = value
        print(f"📄 已加载: {env_file}")


def get_asset_type(file_path: str) -> str:
    """根据文件扩展名自动判断素材类型"""
    ext = Path(file_path).suffix.lower()
    if ext in {".jpg", ".jpeg", ".png", ".webp", ".bmp", ".tiff", ".gif", ".heic", ".heif"}:
        return "Image"
    if ext in {".mp4", ".mov"}:
        return "Video"
    if ext in {".wav", ".mp3"}:
        return "Audio"
    print(f"⚠️  无法识别文件类型: {ext}，默认使用 Image")
    return "Image"


def get_content_type(file_path: str) -> str:
    """获取文件的 MIME 类型"""
    mapping = {
        ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
        ".png": "image/png", ".webp": "image/webp",
        ".bmp": "image/bmp", ".tiff": "image/tiff",
        ".gif": "image/gif", ".heic": "image/heic", ".heif": "image/heif",
        ".mp4": "video/mp4", ".mov": "video/quicktime",
        ".wav": "audio/wav", ".mp3": "audio/mpeg",
    }
    ext = Path(file_path).suffix.lower()
    if ext in mapping:
        return mapping[ext]
    content_type, _ = mimetypes.guess_type(file_path)
    return content_type or "application/octet-stream"


def generate_object_key(file_path: str, prefix: str = "assets") -> str:
    """
    生成 OSS 对象 Key，格式: assets/2026/04/17/abc123_filename.jpg
    用时间 + 内容 MD5 避免覆盖和重复
    """
    file_path = Path(file_path)
    timestamp = time.strftime("%Y/%m/%d")
    md5 = hashlib.md5(file_path.read_bytes()).hexdigest()[:8]
    safe_name = file_path.name.replace(" ", "_")
    return f"{prefix}/{timestamp}/{md5}_{safe_name}"


# ================================================================
#                        OSS 上传
# ================================================================

def upload_to_oss(file_path: str) -> str:
    """
    上传文件到阿里云 OSS，设置公开读，返回公开访问 URL
    """
    try:
        import oss2
    except ImportError:
        print("❌ 请先安装 OSS SDK: pip install oss2")
        sys.exit(1)

    # 验证配置完整性
    required = {
        "OSS_ENDPOINT": OSS_ENDPOINT,
        "OSS_BUCKET": OSS_BUCKET,
        "OSS_ACCESS_KEY_ID": OSS_ACCESS_KEY_ID,
        "OSS_ACCESS_KEY_SECRET": OSS_ACCESS_KEY_SECRET,
    }
    missing = [k for k, v in required.items() if not v]
    if missing:
        print("❌ OSS 配置不完整！缺少以下环境变量：")
        for m in missing:
            print(f"   {m}")
        print("\n请在 .env 文件中配置：")
        print("   OSS_ENDPOINT=oss-cn-hangzhou.aliyuncs.com")
        print("   OSS_BUCKET=your-bucket-name")
        print("   OSS_ACCESS_KEY_ID=your-access-key-id")
        print("   OSS_ACCESS_KEY_SECRET=your-access-key-secret")
        print("   OSS_CUSTOM_DOMAIN=cdn.example.com  （可选）")
        sys.exit(1)

    print(f"\n☁️  正在上传到阿里云 OSS...")
    print(f"   Endpoint: {OSS_ENDPOINT}")
    print(f"   Bucket:   {OSS_BUCKET}")

    auth = oss2.Auth(OSS_ACCESS_KEY_ID, OSS_ACCESS_KEY_SECRET)
    bucket = oss2.Bucket(auth, OSS_ENDPOINT, OSS_BUCKET)

    object_key = generate_object_key(file_path)
    content_type = get_content_type(file_path)
    print(f"   Object:   {object_key}")

    # 上传
    headers = {"Content-Type": content_type}
    with open(file_path, "rb") as f:
        result = bucket.put_object(object_key, f, headers=headers)

    if result.status != 200:
        raise Exception(f"OSS 上传失败: HTTP {result.status}")

    # 设置公开读
    bucket.put_object_acl(object_key, oss2.OBJECT_ACL_PUBLIC_READ)

    # 构造公开 URL
    if OSS_CUSTOM_DOMAIN:
        public_url = f"https://{OSS_CUSTOM_DOMAIN}/{object_key}"
    else:
        public_url = f"https://{OSS_BUCKET}.{OSS_ENDPOINT}/{object_key}"

    print(f"  ✅ 上传成功: {public_url}")
    return public_url


# ================================================================
#                       临时托管（备用）
# ================================================================

def upload_to_tmp_host(file_path: str) -> str:
    """上传到 0x0.st 临时托管，返回公开 URL"""
    print(f"\n📤 上传到临时托管: {file_path}")

    with open(file_path, "rb") as f:
        response = requests.post("https://0x0.st", files={"file": f}, timeout=120)

    if response.status_code != 200:
        # 备选: file.io
        print("  0x0.st 失败，尝试 file.io...")
        with open(file_path, "rb") as f:
            response = requests.post("https://file.io", files={"file": f}, timeout=120)
        if response.status_code == 200:
            data = response.json()
            if data.get("success"):
                url = data["link"]
                print(f"  ✅ 已托管: {url}")
                return url
        raise Exception(f"托管失败: HTTP {response.status_code}")

    url = response.text.strip()
    if not url.startswith("http"):
        raise Exception(f"托管返回异常: {url}")
    print(f"  ✅ 已托管: {url}")
    return url


# ================================================================
#                       素材注册
# ================================================================

def upload_asset(file_url: str, asset_type: str, name: str, token: str) -> dict:
    """调用素材上传接口"""
    print(f"\n📋 注册素材: {name} ({asset_type})")
    print(f"   URL: {file_url}")

    resp = requests.post(
        ASSETS_UPLOAD_URL,
        headers={"Authorization": token, "Content-Type": "application/json"},
        json={"url": file_url, "asset_type": asset_type, "name": name},
        timeout=60,
    )

    if resp.status_code == 401:
        raise Exception("鉴权失败，请检查 token 是否正确")

    return resp.json()


# ================================================================
#                          main
# ================================================================

def main():
    parser = argparse.ArgumentParser(
        description="上传本地文件到素材库",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
环境变量（推荐在 .env 文件中配置）:
  OSS_ENDPOINT=oss-cn-hangzhou.aliyuncs.com
  OSS_BUCKET=your-bucket
  OSS_ACCESS_KEY_ID=your-key
  OSS_ACCESS_KEY_SECRET=your-secret
  OSS_CUSTOM_DOMAIN=cdn.example.com      (可选)
  ASSETS_TOKEN=sk-xxx
  ASSETS_BASE_URL=https://ai.kkidc.com   (默认)

示例:
  python upload_assets.py image.png --oss
  python upload_assets.py video.mp4 --oss --name "演示" --type Video
  python upload_assets.py image.png                    # 临时托管
        """,
    )
    parser.add_argument("file", help="本地文件路径")
    parser.add_argument("--oss", action="store_true", help="使用阿里云 OSS 上传（默认用临时托管）")
    parser.add_argument("--name", default=None, help="素材名称（默认用文件名）")
    parser.add_argument("--type", dest="asset_type", default=None, help="类型: Image/Video/Audio")
    parser.add_argument("--token", default=None, help="API token（优先于环境变量）")

    args = parser.parse_args()

    load_dotenv()

    token = args.token or TOKEN
    if not token:
        print("❌ 未设置 token！请通过以下方式设置：")
        print("   1. 环境变量: ASSETS_TOKEN=sk-xxx")
        print("   2. --token sk-xxx")
        print("   3. 在 .env 文件中: ASSETS_TOKEN=sk-xxx")
        sys.exit(1)

    file_path = Path(args.file)
    if not file_path.exists():
        print(f"❌ 文件不存在: {file_path}")
        sys.exit(1)

    file_size = file_path.stat().st_size
    print(f"📁 {file_path.name}  ({file_size / 1024 / 1024:.2f} MB)")

    asset_type = args.asset_type or get_asset_type(str(file_path))
    name = args.name or file_path.stem
    print(f"🏷️  {asset_type}  |  📝 {name}")

    try:
        file_url = upload_to_oss(str(file_path)) if args.oss else upload_to_tmp_host(str(file_path))
        result = upload_asset(file_url, asset_type, name, token)

        code = result.get("code")
        message = result.get("message", "")
        data = result.get("data", {})

        if code == 0:
            print(f"\n{'=' * 50}")
            print(f"✅ 素材上传成功！")
            print(f"   ID:      {data.get('Id', 'N/A')}")
            print(f"   名称:    {name}")
            print(f"   类型:    {asset_type}")
            print(f"   状态:    {data.get('Status', 'N/A')}")
            print(f"   URL:     {file_url}")
            print(f"{'=' * 50}")
        else:
            print(f"\n❌ 上传失败: [{code}] {message}")
            sys.exit(1)

    except Exception as e:
        print(f"\n❌ 错误: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
