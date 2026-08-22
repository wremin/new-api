#!/usr/bin/env python3
"""创建润元/Seedance 素材组。

用法：
    python docs/examples/create_asset_group.py \
        --api-key "sk-..." \
        --base-url "https://www.metamind.yun" \
        --name "my-assets"

如果素材组已存在，会直接返回已有的 officialId。
"""

from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request
from typing import Any


API_KEY = "sk-"
BASE_URL = "https://www.metamind.yun"
GROUP_NAME = "default-assets"
REQUEST_TIMEOUT = 60


def api_url(base_url: str, path: str) -> str:
    base = base_url.rstrip("/")
    if base.endswith("/v1"):
        return f"{base}{path}"
    return f"{base}/v1{path}"


def request_json(
    api_key: str,
    method: str,
    url: str,
    payload: dict[str, Any] | None = None,
    timeout: int = REQUEST_TIMEOUT,
) -> dict[str, Any]:
    body = None
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Accept": "application/json",
    }

    if payload is not None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"

    req = urllib.request.Request(
        url=url,
        data=body,
        headers=headers,
        method=method,
    )

    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            resp_body = resp.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        err_body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"请求失败：HTTP {exc.code} {exc.reason}\n{err_body}") from exc
    except urllib.error.URLError as exc:
        raise RuntimeError(f"请求失败：{exc.reason}") from exc

    try:
        result = json.loads(resp_body)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"服务端返回的不是合法 JSON：\n{resp_body}") from exc

    return result


def unwrap_response(result: dict[str, Any]) -> dict[str, Any] | list:
    """兼容直接返回数据或使用 data/success 或 data/code 包装的形态。"""
    if "data" in result:
        if "success" in result and result.get("success") is not True:
            msg = result.get("message") or result.get("error") or "unknown error"
            raise RuntimeError(f"接口返回失败：{msg}")
        if "success" not in result:
            code = result.get("code")
            if code is not None and str(code).lower() != "success":
                msg = result.get("message") or result.get("error") or str(code)
                raise RuntimeError(f"接口返回失败：{msg}")
        return result.get("data", {})
    return result


def create_asset_group(api_key: str, base_url: str, name: str) -> str:
    """创建或查找素材组，返回 officialId。"""
    # 先列出已有素材组
    list_resp = request_json(api_key, "GET", api_url(base_url, "/assets/groups"))
    data = unwrap_response(list_resp)
    items: list = data if isinstance(data, list) else (data.get("items") or data.get("groups") or [])

    for item in items:
        if isinstance(item, dict) and item.get("name") == name:
            official_id = item.get("officialId")
            print(f"素材组已存在：{official_id} ({name})")
            return official_id

    # 不存在则创建
    create_resp = request_json(
        api_key,
        "POST",
        api_url(base_url, "/assets/groups"),
        {"name": name, "groupType": "AIGC"},
    )
    created = unwrap_response(create_resp)
    official_id = created.get("officialId")
    if not official_id:
        raise RuntimeError(f"创建素材组未返回 officialId：{json.dumps(created, ensure_ascii=False)}")
    print(f"创建素材组成功：{official_id} ({name})")
    return official_id


def main() -> int:
    parser = argparse.ArgumentParser(description="创建润元/Seedance 素材组")
    parser.add_argument("--api-key", default=API_KEY, help="NewAPI 令牌")
    parser.add_argument("--base-url", default=BASE_URL, help="NewAPI 基地址")
    parser.add_argument("--name", default=GROUP_NAME, help="素材组名称")
    args = parser.parse_args()

    if not args.api_key or args.api_key == "sk-":
        print("错误：请提供 --api-key", file=sys.stderr)
        return 1

    try:
        official_id = create_asset_group(args.api_key, args.base_url, args.name)
        print(official_id)
        return 0
    except (RuntimeError, ValueError) as exc:
        print(f"\n错误：{exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
