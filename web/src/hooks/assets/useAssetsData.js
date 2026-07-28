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

import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@douyinfe/semi-ui';
import { copy, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTableCompactMode } from '../common/useTableCompactMode';
import {
  ASSET_RATE_LIMIT_CODE,
  ASSET_STATUS,
  deleteAsset,
  fetchAssetGroups,
  fetchAssets,
  isAssetChannelError,
  parseAssetError,
  refreshAsset,
} from '../../services/assets';

// 存在处理中素材时的轮询间隔（毫秒）
export const ASSET_POLL_INTERVAL = 15000;

export const useAssetsData = () => {
  const { t } = useTranslation();

  // Basic state
  const [assets, setAssets] = useState([]);
  const [loading, setLoading] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [assetCount, setAssetCount] = useState(0);

  // 素材组（用于筛选下拉与表格中的名称展示）
  const [groups, setGroups] = useState([]);
  const [groupsLoading, setGroupsLoading] = useState(false);

  // 渠道未配置 / 渠道不唯一时的空状态
  const [channelError, setChannelError] = useState(null);

  // Form state
  const [formApi, setFormApi] = useState(null);
  const formInitValues = {
    groupId: '',
    status: '',
    assetType: '',
    keyword: '',
  };

  // Modal state
  const [showUploadModal, setShowUploadModal] = useState(false);

  // 正在单条刷新的素材 officialId
  const [refreshingId, setRefreshingId] = useState('');

  // Compact mode
  const [compactMode, setCompactMode] = useTableCompactMode('assets');

  // 统一的错误处理：渠道类错误转为空状态，其余弹出提示
  const handleAssetError = (error, options = {}) => {
    const { silent = false } = options;
    const parsed = parseAssetError(error);

    if (isAssetChannelError(parsed)) {
      setChannelError(parsed);
      return parsed;
    }

    if (silent) return parsed;

    if (parsed.code === ASSET_RATE_LIMIT_CODE) {
      showError(t('素材接口请求过于频繁，请稍后再试'));
      return parsed;
    }

    showError(parsed.message || t('素材接口请求失败'));
    return parsed;
  };

  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};
    return {
      groupId: formValues.groupId || '',
      status: formValues.status || '',
      assetType: formValues.assetType || '',
      keyword: formValues.keyword || '',
    };
  };

  const enrichAssets = (items) =>
    (items || []).map((item, index) => ({
      ...item,
      key: `${item.officialId || item.id || index}`,
    }));

  const loadAssets = async (page = activePage, size = pageSize, options = {}) => {
    const { refresh = false, silent = false } = options;
    if (!silent) setLoading(true);
    try {
      const filters = getFormValues();
      const payload = await fetchAssets({
        page_num: page,
        page_size: size,
        ...filters,
        ...(refresh ? { refresh: 'true' } : {}),
      });
      setAssets(enrichAssets(payload.items));
      setAssetCount(payload.total || 0);
      setActivePage(payload.page_num || page);
      setPageSize(payload.page_size || size);
      setChannelError(null);
    } catch (error) {
      handleAssetError(error, { silent });
    } finally {
      if (!silent) setLoading(false);
    }
  };

  const loadGroups = async () => {
    setGroupsLoading(true);
    try {
      const list = await fetchAssetGroups();
      setGroups(list);
    } catch (error) {
      handleAssetError(error, { silent: true });
    } finally {
      setGroupsLoading(false);
    }
  };

  // Page handlers
  const handlePageChange = (page) => {
    loadAssets(page, pageSize).then();
  };

  const handlePageSizeChange = async (size) => {
    await loadAssets(1, size);
  };

  // 查询（表单提交）
  const refresh = async () => {
    await loadAssets(1, pageSize);
  };

  // 工具栏「刷新」按钮：带 refresh=true，让服务端先同步一次上游状态
  const refreshWithSync = async () => {
    await loadAssets(activePage, pageSize, { refresh: true });
    await loadGroups();
  };

  // 单条刷新状态
  const refreshAssetStatus = async (record) => {
    if (!record?.officialId) return;
    setRefreshingId(record.officialId);
    try {
      await refreshAsset(record.officialId);
      showSuccess(t('已刷新素材状态'));
      await loadAssets(activePage, pageSize, { silent: true });
    } catch (error) {
      handleAssetError(error);
    } finally {
      setRefreshingId('');
    }
  };

  // 删除素材
  const removeAsset = async (record) => {
    if (!record?.officialId) return;
    try {
      await deleteAsset(record.officialId);
      showSuccess(t('删除成功'));
      await loadAssets(activePage, pageSize);
    } catch (error) {
      handleAssetError(error);
    }
  };

  // 点击素材组名称，按该素材组筛选
  const filterByGroup = (groupId) => {
    if (!groupId || !formApi) return;
    formApi.setValue('groupId', groupId);
    loadAssets(1, pageSize).then();
  };

  const copyText = async (text) => {
    if (!text) return;
    if (await copy(text)) {
      showSuccess(t('已复制：') + text);
    } else {
      Modal.error({ title: t('无法复制到剪贴板，请手动复制'), content: text });
    }
  };

  // 素材组下拉选项
  const groupOptions = useMemo(
    () =>
      groups.map((group) => ({
        label: group.region
          ? `${group.name} (${group.region})`
          : group.name || group.officialId,
        value: group.officialId,
      })),
    [groups],
  );

  // officialId / id 双向索引，便于把素材上的 groupId 还原成素材组
  const groupMap = useMemo(() => {
    const map = {};
    groups.forEach((group) => {
      if (group.officialId) map[group.officialId] = group;
      if (group.id !== undefined && group.id !== null) map[`${group.id}`] = group;
    });
    return map;
  }, [groups]);

  const hasPendingAssets = useMemo(
    () => assets.some((item) => item.status === ASSET_STATUS.PROCESSING),
    [assets],
  );

  // 保存最新的加载函数，避免轮询定时器捕获到过期闭包
  const loadAssetsRef = useRef(loadAssets);
  useEffect(() => {
    loadAssetsRef.current = loadAssets;
  });

  // 初始化
  useEffect(() => {
    loadGroups().then();
    loadAssets(1, ITEMS_PER_PAGE).then();
  }, []);

  // 轮询：当前页存在处理中的素材时，每 15s 带 refresh=true 拉取一次
  useEffect(() => {
    if (!hasPendingAssets || channelError) return undefined;
    const timer = setInterval(() => {
      loadAssetsRef.current?.(activePage, pageSize, {
        refresh: true,
        silent: true,
      });
    }, ASSET_POLL_INTERVAL);
    return () => clearInterval(timer);
  }, [hasPendingAssets, channelError, activePage, pageSize]);

  return {
    // Basic state
    assets,
    loading,
    activePage,
    pageSize,
    assetCount,
    hasPendingAssets,

    // Groups
    groups,
    groupsLoading,
    groupOptions,
    groupMap,
    loadGroups,

    // Channel empty state
    channelError,

    // Form state
    formApi,
    setFormApi,
    formInitValues,
    getFormValues,

    // Modal state
    showUploadModal,
    setShowUploadModal,

    // Compact mode
    compactMode,
    setCompactMode,

    // Row level state
    refreshingId,

    // Functions
    loadAssets,
    handlePageChange,
    handlePageSizeChange,
    refresh,
    refreshWithSync,
    refreshAssetStatus,
    removeAsset,
    filterByGroup,
    copyText,
    handleAssetError,

    // Translation
    t,
  };
};
