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

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@douyinfe/semi-ui';
import { copy, showError, showSuccess } from '../../helpers';
import { useTableCompactMode } from '../common/useTableCompactMode';
import {
  ASSET_RATE_LIMIT_CODE,
  createAssetGroup,
  fetchAssetGroups,
  isAssetChannelError,
  parseAssetError,
} from '../../services/assets';

export const useAssetGroupsData = () => {
  const { t } = useTranslation();

  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [channelError, setChannelError] = useState(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [compactMode, setCompactMode] = useTableCompactMode('assetGroups');

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

  const loadGroups = async () => {
    setLoading(true);
    try {
      const list = await fetchAssetGroups();
      setGroups(list);
      setChannelError(null);
    } catch (error) {
      handleAssetError(error);
    } finally {
      setLoading(false);
    }
  };

  const createGroup = async (payload) => {
    setCreating(true);
    try {
      await createAssetGroup(payload);
      showSuccess(t('创建成功'));
      setShowCreateModal(false);
      await loadGroups();
      return true;
    } catch (error) {
      handleAssetError(error);
      return false;
    } finally {
      setCreating(false);
    }
  };

  const copyText = async (text) => {
    if (!text) return;
    if (await copy(text)) {
      showSuccess(t('已复制：') + text);
    } else {
      Modal.error({ title: t('无法复制到剪贴板，请手动复制'), content: text });
    }
  };

  useEffect(() => {
    loadGroups().then();
  }, []);

  return {
    groups,
    loading,
    creating,
    channelError,
    showCreateModal,
    setShowCreateModal,
    compactMode,
    setCompactMode,
    loadGroups,
    createGroup,
    copyText,
    t,
  };
};
