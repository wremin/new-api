/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  DEFAULT_ASSET_CAPABILITIES,
  getAssetCapabilities,
  isAssetChannelError,
  parseAssetError,
} from '../../services/assets';

/**
 * 拉取一次上游素材能力（/v1/assets/capabilities），用于按上游降级前端功能。
 *
 * 在页面（pages/Assets）中调用一次，再把 capabilities 透传给两个标签页，
 * 与仓库里「hook 放在页面层、结果向下透传」的写法保持一致。
 *
 * 失败处理分两种：
 * - 渠道未配置 / 渠道不唯一：通过 channelError 暴露，页面立即渲染空状态，
 *   不能退回默认能力，否则会渲染出注定 501 的按钮和错误的上游名称；
 * - 其他错误（网络抖动等）：保留默认能力，不整页置空。
 */
export const useAssetCapabilities = () => {
  const [capabilities, setCapabilities] = useState(DEFAULT_ASSET_CAPABILITIES);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [channelError, setChannelError] = useState(null);
  // 组件卸载后不再 setState
  const mountedRef = useRef(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getAssetCapabilities();
      if (!mountedRef.current) return;
      setCapabilities(data);
      setError(null);
      setChannelError(null);
    } catch (err) {
      if (!mountedRef.current) return;
      const parsed = parseAssetError(err);
      setCapabilities(DEFAULT_ASSET_CAPABILITIES);
      setError(parsed);
      setChannelError(isAssetChannelError(parsed) ? parsed : null);
    } finally {
      if (mountedRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    load().then();
    return () => {
      mountedRef.current = false;
    };
  }, [load]);

  return { capabilities, loading, error, channelError, reload: load };
};
