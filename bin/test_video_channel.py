#!/usr/bin/env python3
"""
Stelloria 视频生成通道测试脚本
用法: python test_video_channel.py --api-key sk-xxx [--model seedance-2.0] [--prompt "..."]
"""
import argparse
import json
import sys
import time

import requests

BASE_URL = "https://www.metamind.yun"


def submit_task(api_key: str, model: str, prompt: str, **kwargs):
    """提交视频生成任务"""
    url = f"{BASE_URL}/v1/video/generations"
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }

    metadata = {}
    if kwargs.get("duration"):
        metadata["duration"] = kwargs["duration"]
    if kwargs.get("aspect_ratio"):
        metadata["aspect_ratio"] = kwargs["aspect_ratio"]
    if kwargs.get("resolution"):
        metadata["resolution"] = kwargs["resolution"]
    if kwargs.get("fps"):
        metadata["fps"] = kwargs["fps"]
    if kwargs.get("seed") is not None:
        metadata["seed"] = kwargs["seed"]
    if kwargs.get("image_url"):
        metadata["image_url"] = kwargs["image_url"]

    payload = {
        "model": model,
        "prompt": prompt,
        "metadata": metadata,
    }

    print(f"[提交] POST {url}")
    print(f"[提交] payload: {json.dumps(payload, ensure_ascii=False, indent=2)}")

    resp = requests.post(url, headers=headers, json=payload, timeout=60)
    print(f"[提交] HTTP {resp.status_code}")

    try:
        data = resp.json()
    except Exception:
        print(f"[提交] 响应解析失败: {resp.text[:500]}")
        return None

    print(f"[提交] 响应: {json.dumps(data, ensure_ascii=False, indent=2)}")

    if resp.status_code != 200:
        print(f"[错误] 提交失败")
        return None

    # 尝试从响应中提取 task_id
    task_id = data.get("task_id") or data.get("id") or data.get("TaskID")
    if not task_id:
        # 可能是 OpenAI 兼容格式
        task_id = data.get("id")

    if task_id:
        print(f"[提交] 任务ID: {task_id}")
    else:
        print(f"[提交] 无法提取任务ID")

    return task_id


def poll_task(api_key: str, task_id: str, max_wait: int = 300, interval: int = 10):
    """轮询任务状态"""
    url = f"{BASE_URL}/v1/video/generations/{task_id}"
    headers = {
        "Authorization": f"Bearer {api_key}",
    }

    start = time.time()
    attempt = 0

    while time.time() - start < max_wait:
        attempt += 1
        elapsed = int(time.time() - start)

        try:
            resp = requests.get(url, headers=headers, timeout=30)
            data = resp.json()
        except requests.exceptions.JSONDecodeError:
            print(f"[轮询 #{attempt}] {elapsed}s - HTTP {resp.status_code}, 响应解析失败")
            time.sleep(interval)
            continue
        except Exception as e:
            print(f"[轮询 #{attempt}] {elapsed}s - 请求异常: {e}")
            time.sleep(interval)
            continue

        status = data.get("status", "unknown")
        progress = data.get("progress", "")
        task_id_resp = data.get("task_id") or data.get("id", "")

        print(
            f"[轮询 #{attempt}] {elapsed}s - status={status}"
            + (f" progress={progress}" if progress else "")
        )

        if status in ("succeeded", "completed", "success"):
            print(f"\n{'='*60}")
            print(f"[成功] 视频生成完成！")
            print(f"{'='*60}")

            # 提取视频 URL
            video_url = (
                data.get("video_url")
                or data.get("result_url")
                or data.get("url")
                or (data.get("result", {}) or {}).get("video_url")
                or (data.get("result", {}) or {}).get("result_url")
            )
            if video_url:
                print(f"视频地址: {video_url}")

            # 打印完整响应
            print(f"\n完整响应:")
            print(json.dumps(data, ensure_ascii=False, indent=2))
            return True

        elif status in ("failed", "error"):
            print(f"\n{'='*60}")
            print(f"[失败] 视频生成失败")
            print(f"{'='*60}")
            error_msg = (
                data.get("fail_reason")
                or data.get("error")
                or data.get("reason")
                or data.get("message")
                or "未知错误"
            )
            if isinstance(error_msg, dict):
                error_msg = json.dumps(error_msg, ensure_ascii=False)
            print(f"错误信息: {error_msg}")
            print(f"\n完整响应:")
            print(json.dumps(data, ensure_ascii=False, indent=2))
            return False

        time.sleep(interval)

    print(f"\n[超时] 等待 {max_wait}s 后任务仍未完成")
    return False


def test_channel_health(api_key: str):
    """基础连通性测试"""
    url = f"{BASE_URL}/api/status"
    try:
        resp = requests.get(url, timeout=10)
        print(f"[连通] {BASE_URL} - HTTP {resp.status_code}")
        return resp.status_code == 200
    except Exception as e:
        print(f"[连通] {BASE_URL} - 连接失败: {e}")
        return False


def main():
    parser = argparse.ArgumentParser(description="测试视频生成通道")
    parser.add_argument("--api-key", "-k", required=True, help="API Key")
    parser.add_argument("--model", "-m", default="seedance-2.0", help="模型名称")
    parser.add_argument(
        "--prompt",
        "-p",
        default="一只金毛犬在海边奔跑，浪花飞溅，阳光明媚",
        help="视频提示词",
    )
    parser.add_argument("--duration", "-d", default="5s", help="视频时长: 5s, 10s")
    parser.add_argument("--aspect-ratio", "-r", default="16:9", help="宽高比: 16:9, 9:16, 1:1")
    parser.add_argument("--resolution", default="720p", help="分辨率: 720p, 1080p")
    parser.add_argument("--fps", type=int, default=24, help="帧率: 24, 30")
    parser.add_argument("--seed", type=int, default=None, help="随机种子")
    parser.add_argument("--image-url", default=None, help="参考图片URL（图生视频）")
    parser.add_argument("--max-wait", type=int, default=300, help="最大等待时间(秒)")
    parser.add_argument("--interval", type=int, default=10, help="轮询间隔(秒)")

    args = parser.parse_args()

    print(f"{'='*60}")
    print(f"  视频生成通道测试")
    print(f"  网关: {BASE_URL}")
    print(f"  模型: {args.model}")
    print(f"  提示词: {args.prompt}")
    print(f"{'='*60}\n")

    # 1. 连通性测试
    if not test_channel_health(args.api_key):
        print("[警告] 网关连通性检查失败，但继续尝试...")

    # 2. 提交任务
    task_id = submit_task(
        args.api_key,
        args.model,
        args.prompt,
        duration=args.duration,
        aspect_ratio=args.aspect_ratio,
        resolution=args.resolution,
        fps=args.fps,
        seed=args.seed,
        image_url=args.image_url,
    )

    if not task_id:
        print("\n[结果] 任务提交失败，通道异常")
        sys.exit(1)

    # 3. 轮询等待
    success = poll_task(args.api_key, task_id, args.max_wait, args.interval)

    if success:
        print("\n[结果] 通道正常")
        sys.exit(0)
    else:
        print("\n[结果] 通道异常或超时")
        sys.exit(1)


if __name__ == "__main__":
    main()
