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

import React, { useEffect, useState } from 'react';
import {
  Banner,
  InputNumber,
  Modal,
  Radio,
  RadioGroup,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../../../helpers';

const { Text } = Typography;

// 走通用的 /api/option/ 接口（RootAuth），返回的是 { success, message } 结构，
// 与 /v1/assets 的 { error: {...} } 不同，因此不能带 skipErrorHandler
const PROVIDER_OPTION_KEY = 'assets_setting.provider';
const CHANNEL_ID_OPTION_KEY = 'assets_setting.channel_id';

const AssetSettingsModal = ({
  visible,
  onCancel,
  onSaved,
  currentProvider,
  t,
}) => {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [provider, setProvider] = useState('auto');
  const [channelId, setChannelId] = useState(0);
  // 已保存的值，用于判断本次是否真的改动了配置
  const [saved, setSaved] = useState({ provider: 'auto', channelId: 0 });

  const loadOptions = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/option/');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('读取素材库设置失败'));
        return;
      }
      const options = {};
      (data || []).forEach((item) => {
        options[item.key] = item.value;
      });
      const nextProvider = options[PROVIDER_OPTION_KEY] || 'auto';
      const parsed = parseInt(options[CHANNEL_ID_OPTION_KEY], 10);
      const nextChannelId = Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
      setProvider(nextProvider);
      setChannelId(nextChannelId);
      setSaved({ provider: nextProvider, channelId: nextChannelId });
    } catch (error) {
      showError(t('读取素材库设置失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!visible) return;
    loadOptions().then();
  }, [visible]);

  const persist = async () => {
    const updates = [];
    if (provider !== saved.provider) {
      updates.push({ key: PROVIDER_OPTION_KEY, value: provider });
    }
    if (channelId !== saved.channelId) {
      updates.push({ key: CHANNEL_ID_OPTION_KEY, value: String(channelId) });
    }
    if (updates.length === 0) {
      onCancel?.();
      return;
    }

    setSaving(true);
    try {
      for (const item of updates) {
        const res = await API.put('/api/option/', item);
        if (!res.data?.success) {
          showError(res.data?.message || t('保存失败，请重试'));
          return;
        }
      }
      setSaved({ provider, channelId });
      showSuccess(t('保存成功'));
      onCancel?.();
      // 让上游能力、两个标签页的数据都按新配置重新拉取
      await onSaved?.();
    } catch (error) {
      showError(t('保存失败，请重试'));
    } finally {
      setSaving(false);
    }
  };

  const handleOk = () => {
    // 只有 provider 本身变化才二次确认：
    // 改渠道 ID 通常只是换一个同上游的账号，不必每次都弹这个警告。
    // 尚未有可用上游时（渠道未配置的首次配置）同样不需要吓人的确认。
    const providerChanged = provider !== saved.provider;
    if (!providerChanged || !currentProvider) {
      persist().then();
      return;
    }
    Modal.confirm({
      title: t('确认切换素材上游？'),
      content: t(
        '切换后，此前在旧上游创建的素材将不可用：引用它们的 asset:// 请求会返回 asset_provider_mismatch（HTTP 409），并在素材列表中标记为「上游已切换」。唯一的处理办法是删除这些本地记录。',
      ),
      centered: true,
      okType: 'danger',
      okText: t('确认切换'),
      cancelText: t('取消'),
      onOk: persist,
    });
  };

  return (
    <Modal
      title={t('素材库设置')}
      visible={visible}
      onCancel={onCancel}
      onOk={handleOk}
      confirmLoading={saving}
      okText={t('保存')}
      cancelText={t('取消')}
      maskClosable={false}
    >
      <Spin spinning={loading}>
        <div className='flex flex-col gap-3'>
          {/* 上游 */}
          <div>
            <div className='mb-1'>
              <Text size='small'>{t('上游')}</Text>
            </div>
            <RadioGroup
              type='button'
              value={provider}
              onChange={(e) => setProvider(e.target.value)}
            >
              <Radio value='auto'>{t('自动探测')}</Radio>
              <Radio value='seegen'>seegen</Radio>
              <Radio value='stelloria'>stelloria</Radio>
            </RadioGroup>
            <Text type='tertiary' size='small' className='block mt-1'>
              {t('自动探测会根据素材渠道的 base_url 判断上游。')}
            </Text>
          </div>

          {/* 素材渠道 ID */}
          <div>
            <div className='mb-1'>
              <Text size='small'>{t('素材渠道 ID')}</Text>
            </div>
            <InputNumber
              value={channelId}
              onChange={(value) => setChannelId(Number(value) || 0)}
              min={0}
              precision={0}
              className='w-full'
            />
            <Text type='tertiary' size='small' className='block mt-1'>
              {t('0 表示自动探测唯一启用的 Seedance 渠道。')}
            </Text>
          </div>

          <Banner
            type='warning'
            closeIcon={null}
            className='!rounded-lg'
            description={t(
              '切换上游会使此前在旧上游创建的素材失效，保存前请确认影响范围。',
            )}
          />
        </div>
      </Spin>
    </Modal>
  );
};

export default AssetSettingsModal;
