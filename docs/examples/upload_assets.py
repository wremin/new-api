#!/usr/bin/env python3
"""
素材库上传脚本 —— 通过 new-api 中转站上传素材，拿到可用于视频生成的 asset:// 引用。

只依赖标准库，不需要 pip install。

用法:
  # 新建素材组并上传（最常用）
  python3 upload_assets.py --group-name "产品素材" https://example.com/a.jpg

  # 上传到已有素材组，支持多个 URL
  python3 upload_assets.py --group-id group-abc123 https://a.jpg https://b.mp4

  # 查看素材组 / 素材列表
  python3 upload_assets.py --list-groups
  python3 upload_assets.py --list

  # 删除素材
  python3 upload_assets.py --delete asset-abc123

脚本会先查询 /v1/assets/capabilities，按当前上游的能力自动决定
素材组用 region 还是 groupType 分类，因此换上游不需要改脚本。
"""

import argparse
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

# ======================== 配置区域 ========================

# new-api 的地址，Base URL 本身包含 /v1
BASE_URL = "https://www.metamind.yun/v1"

# new-api 控制台创建的下游令牌
API_KEY = "sk-kYNUvTlYbW81hvjpsHJEKv0ik0R5H04J47sp0u8C3Ii5e6h6"

# 轮询审核状态的间隔与上限
POLL_INTERVAL_SECONDS = 5
MAX_WAIT_SECONDS = 300

# ========================================================


class ApiError(Exception):
    """封装 new-api 返回的 {"error":{...}} 错误体。"""

    def __init__(self, status, code, message):
        self.status = status
        self.code = code
        self.message = message
        super().__init__(f"[HTTP {status}] {code}: {message}")


def request_json(method, path, body=None, params=None):
    """
    向 new-api 发一个请求并解析 JSON。

    错误统一抛 ApiError，把 error.code 带出来——上层可以据此区分
    asset_not_found / asset_not_active / assets_channel_not_configured 等情况。
    """
    url = f"{BASE_URL.rstrip('/')}{path}"
    if params:
        url = f"{url}?{urllib.parse.urlencode(params)}"

    data = None
    headers = {
        "Authorization": f"Bearer {API_KEY}",
        "Accept": "application/json",
    }
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"

    req = urllib.request.Request(url=url, data=data, headers=headers, method=method)

    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            text = resp.read().decode("utf-8")
            return json.loads(text) if text else {}
    except urllib.error.HTTPError as exc:
        text = exc.read().decode("utf-8", errors="replace")
        try:
            payload = json.loads(text)
            err = payload.get("error") or {}
            raise ApiError(exc.code, err.get("code", ""), err.get("message", text))
        except json.JSONDecodeError:
            raise ApiError(exc.code, "", text)


# ======================== 能力探测 ========================


def get_capabilities():
    """
    查询当前上游支持哪些能力。

    返回示例:
      {"provider":"metamind","batchCreate":true,"excelTemplate":false,
       "regions":false,"groupTypes":["AIGC","LivenessFace"],"batchMaxItems":50}

    regions=true  -> 素材组用 region 分类（cn / intl）
    groupTypes 非空 -> 素材组用 groupType 分类
    两者互斥，传错那个会被明确拒绝。
    """
    return request_json("GET", "/assets/capabilities")


def describe_capabilities(caps):
    lines = [f"上游: {caps.get('provider', '未知')}"]
    if caps.get("regions"):
        lines.append("素材组分类: region（cn / intl）")
    elif caps.get("groupTypes"):
        lines.append(f"素材组分类: groupType（{' / '.join(caps['groupTypes'])}）")
    else:
        lines.append("素材组分类: 无")
    lines.append(f"批量上传: {'支持' if caps.get('batchCreate') else '不支持'}"
                 f"（单批上限 {caps.get('batchMaxItems', 50)} 条）")
    lines.append(f"Excel 模板: {'支持' if caps.get('excelTemplate') else '不支持'}")
    return "\n".join(f"  {line}" for line in lines)


# ======================== 素材组 ========================


def create_group(name, caps, description=None, region=None, group_type=None):
    """
    创建素材组。

    region 与 groupType 分属不同上游、互斥，按能力集自动选择：
    未显式指定时，有 groupTypes 就用第一个，有 regions 就用 cn。
    """
    payload = {"name": name}
    if description:
        payload["description"] = description

    if caps.get("regions"):
        payload["region"] = region or "cn"
    elif caps.get("groupTypes"):
        payload["groupType"] = group_type or caps["groupTypes"][0]

    return request_json("POST", "/assets/groups", body=payload)


def list_groups():
    return request_json("GET", "/assets/groups")


# ======================== 素材 ========================


def upload_one(group_id, url, name=None, asset_type=None):
    payload = {"groupId": group_id, "url": url}
    if name:
        payload["name"] = name
    if asset_type:
        payload["assetType"] = asset_type
    return request_json("POST", "/assets", body=payload)


def upload_batch(group_id, urls):
    """
    批量上传。上游没有原生批量接口时，new-api 会退化成循环单条创建，
    响应形态保持一致（只是 batchId 为空）。
    """
    items = [{"groupId": group_id, "url": u} for u in urls]
    return request_json("POST", "/assets/batch", body=items)


def get_asset(official_id):
    """查询单个素材。这个接口同时会把最新状态同步回 new-api 本地。"""
    return request_json("GET", f"/assets/{urllib.parse.quote(official_id)}")


def list_assets(group_id=None, status=None, refresh=True):
    params = {"page_size": 50}
    if group_id:
        params["groupId"] = group_id
    if status:
        params["status"] = status
    if refresh:
        # refresh=true 才会回源同步审核状态，默认只查本地表以保证列表接口够快
        params["refresh"] = "true"
    return request_json("GET", "/assets", params=params)


def delete_asset(official_id):
    return request_json("DELETE", f"/assets/{urllib.parse.quote(official_id)}")


def wait_until_active(official_id):
    """
    轮询到素材审核结束。

    只有 Active 状态的素材才能在生成任务中引用，
    引用 Processing 的会被 new-api 拦下并返回 asset_not_active。
    """
    deadline = time.monotonic() + MAX_WAIT_SECONDS
    while time.monotonic() < deadline:
        asset = get_asset(official_id)
        status = asset.get("status", "")
        if status == "Active":
            return asset
        if status == "Failed":
            reason = asset.get("failReason") or "上游未提供原因"
            print(f"  ✗ {official_id} 审核未通过: {reason}", file=sys.stderr)
            return asset
        print(f"  … {official_id} {status or '状态未知'}，{POLL_INTERVAL_SECONDS} 秒后重试")
        time.sleep(POLL_INTERVAL_SECONDS)

    print(f"  ! {official_id} 等待超过 {MAX_WAIT_SECONDS} 秒，稍后可再查", file=sys.stderr)
    return get_asset(official_id)


# ======================== 命令行 ========================


def cmd_list_groups():
    groups = list_groups()
    if not groups:
        print("还没有素材组。用 --group-name 创建一个。")
        return 0
    print(f"{'officialId':<28} {'名称':<20} {'分类':<14} 素材数")
    for g in groups:
        kind = g.get("region") or g.get("groupType") or "-"
        count = (g.get("_count") or {}).get("assets", 0)
        print(f"{g['officialId']:<28} {g.get('name', ''):<20} {kind:<14} {count}")
    return 0


def cmd_list_assets(args):
    result = list_assets(group_id=args.group_id, status=args.status)
    items = result.get("items", [])
    if not items:
        print("没有符合条件的素材。")
        return 0
    print(f"共 {result.get('total', len(items))} 条\n")
    for a in items:
        line = f"{a['officialId']:<28} {a.get('status', ''):<12} {a.get('name', '')}"
        print(line)
        print(f"  引用: {a.get('assetRef', '')}")
        if a.get("failReason"):
            print(f"  失败原因: {a['failReason']}")
    return 0


def cmd_upload(args, caps):
    group_id = args.group_id

    if args.group_name:
        group = create_group(
            args.group_name, caps,
            description=args.description,
            region=args.region,
            group_type=args.group_type,
        )
        group_id = group["officialId"]
        kind = group.get("region") or group.get("groupType") or "-"
        print(f"✓ 已创建素材组 {group_id}（{group.get('name')} / {kind}）\n")

    if not group_id:
        print("请用 --group-id 指定素材组，或用 --group-name 新建一个。", file=sys.stderr)
        return 2

    urls = args.urls
    official_ids = []

    if len(urls) == 1:
        asset = upload_one(group_id, urls[0], name=args.name)
        official_ids.append(asset["officialId"])
        print(f"✓ 已提交 {asset['officialId']}（{asset.get('status')}）")
    else:
        max_items = caps.get("batchMaxItems", 50)
        if len(urls) > max_items:
            print(f"单批最多 {max_items} 条，本次给了 {len(urls)} 条。", file=sys.stderr)
            return 2
        result = upload_batch(group_id, urls)
        print(f"✓ 已提交 {result.get('total', len(urls))} 条")
        for item in result.get("results", []):
            idx = item.get("index", -1)
            src = urls[idx] if 0 <= idx < len(urls) else "?"
            if item.get("status") == "ok":
                official_ids.append(item["officialId"])
                print(f"  [{idx}] ok    {item['officialId']}  <- {src}")
            else:
                print(f"  [{idx}] error {item.get('error', '')}  <- {src}", file=sys.stderr)

    if not official_ids or args.no_wait:
        return 0

    print("\n等待审核…")
    ready = []
    for official_id in official_ids:
        asset = wait_until_active(official_id)
        if asset.get("status") == "Active":
            ready.append(asset)

    if not ready:
        print("\n没有素材通过审核。", file=sys.stderr)
        return 1

    print(f"\n{'=' * 60}")
    print(f"✓ {len(ready)} 个素材可用，把下面的引用填进生成任务即可：\n")
    for a in ready:
        print(f"  {a['assetRef']}   ({a.get('name') or a['officialId']})")
    print(f"{'=' * 60}")
    return 0


def main():
    parser = argparse.ArgumentParser(
        description="上传素材到 new-api 素材库并获取 asset:// 引用",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  # 新建素材组并上传一张图，等待审核通过后打印 asset:// 引用
  python3 upload_assets.py --group-name "产品素材" https://example.com/a.jpg

  # 上传到已有素材组，多个 URL 走批量
  python3 upload_assets.py --group-id group-abc123 https://a.jpg https://b.jpg

  # 只提交不等审核
  python3 upload_assets.py --group-id group-abc123 --no-wait https://a.jpg

  # 查看
  python3 upload_assets.py --list-groups
  python3 upload_assets.py --list --status Active
        """,
    )
    parser.add_argument("urls", nargs="*", help="素材的公网可访问 URL，可给多个")
    parser.add_argument("--group-id", help="上传到已有素材组的 officialId")
    parser.add_argument("--group-name", help="新建素材组并上传到它")
    parser.add_argument("--description", help="新建素材组时的描述")
    parser.add_argument("--region", help="仅当上游有区域概念时生效：cn / intl")
    parser.add_argument("--group-type", help="仅当上游有素材组类型时生效，如 AIGC")
    parser.add_argument("--name", help="素材名称，仅单个上传时生效")
    parser.add_argument("--no-wait", action="store_true", help="提交后不等待审核")
    parser.add_argument("--list-groups", action="store_true", help="列出素材组")
    parser.add_argument("--list", action="store_true", help="列出素材")
    parser.add_argument("--status", help="列表筛选：Processing / Active / Failed")
    parser.add_argument("--delete", metavar="ASSET_ID", help="删除指定素材")

    args = parser.parse_args()

    if not API_KEY or "替换" in API_KEY:
        print("请先在脚本顶部填写 API_KEY。", file=sys.stderr)
        return 2

    try:
        if args.delete:
            delete_asset(args.delete)
            print(f"✓ 已删除 {args.delete}")
            return 0

        if args.list_groups:
            return cmd_list_groups()

        if args.list:
            return cmd_list_assets(args)

        if not args.urls:
            parser.print_help()
            return 2

        caps = get_capabilities()
        print("当前素材库能力：")
        print(describe_capabilities(caps))
        print()
        return cmd_upload(args, caps)

    except ApiError as exc:
        # 把已知错误码翻译成人话，省去查文档
        hints = {
            "assets_channel_not_configured": "管理员还没配置素材渠道，去控制台「素材库 → 素材库设置」配一下。",
            "assets_channel_ambiguous": "存在多个可用渠道，需要管理员显式指定素材渠道 ID。",
            "asset_not_found": "素材不存在，或不属于当前令牌所属的用户。",
            "asset_not_active": "素材还没通过审核，只有 Active 状态的素材能被引用。",
            "asset_unsupported_by_provider": "当前上游不支持这个能力，先看 /v1/assets/capabilities。",
            "asset_provider_mismatch": "这个素材是在切换上游之前创建的，已经失效，只能删除本地记录。",
            "assets_rate_limit_exceeded": "触发限流，稍后再试。",
        }
        print(f"\n✗ {exc}", file=sys.stderr)
        if exc.code in hints:
            print(f"  {hints[exc.code]}", file=sys.stderr)
        return 1
    except urllib.error.URLError as exc:
        print(f"\n✗ 网络错误: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
