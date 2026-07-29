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

import React, { useState } from 'react';
import { Button, Tabs, TabPane, Tag } from '@douyinfe/semi-ui';
import { Settings } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import AssetsTablePage from '../../components/table/assets';
import AssetGroupsPage from '../../components/table/assets/AssetGroupsPage';
import AssetsChannelEmpty from '../../components/table/assets/AssetsChannelEmpty';
import AssetSettingsModal from '../../components/table/assets/modals/AssetSettingsModal';
import { useAssetCapabilities } from '../../hooks/assets/useAssetCapabilities';
import { isRoot } from '../../helpers';

const Assets = () => {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState('assets');
  const [showSettings, setShowSettings] = useState(false);
  // 保存设置后自增，用于强制两个标签页重新挂载并按新配置重新拉取数据
  const [reloadKey, setReloadKey] = useState(0);
  // 只有 root 能读写 /api/option/，其余角色仍是只读的上游标签
  const canManage = isRoot();

  // 上游能力只拉取一次，两个标签页共用
  const {
    capabilities,
    loading: capabilitiesLoading,
    channelError,
    reload: reloadCapabilities,
  } = useAssetCapabilities();

  const handleSettingsSaved = async () => {
    await reloadCapabilities();
    setReloadKey((key) => key + 1);
  };

  const openSettings = canManage ? () => setShowSettings(true) : undefined;

  // 能力接口就已经报渠道错误时，两个标签页直接置为空状态，
  // 不必等各自的列表请求再失败一次
  const channelEmpty = channelError ? (
    <AssetsChannelEmpty
      channelError={channelError}
      onConfigure={openSettings}
      t={t}
    />
  ) : null;

  return (
    <div className='mt-[60px] px-2'>
      {canManage ? (
        <AssetSettingsModal
          visible={showSettings}
          onCancel={() => setShowSettings(false)}
          onSaved={handleSettingsSaved}
          currentProvider={capabilities.provider}
          t={t}
        />
      ) : null}

      <Tabs
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key)}
        type='card'
        tabBarExtraContent={
          <div className='flex items-center gap-2'>
            {capabilities.provider ? (
              <Tag color='white' shape='circle'>
                {t('上游：{{provider}}', { provider: capabilities.provider })}
              </Tag>
            ) : null}
            {canManage ? (
              <Button
                type='tertiary'
                theme='borderless'
                size='small'
                icon={<Settings size={14} />}
                onClick={() => setShowSettings(true)}
              >
                {t('素材库设置')}
              </Button>
            ) : null}
          </div>
        }
      >
        <TabPane tab={t('素材列表')} itemKey='assets'>
          {channelEmpty || (
            <AssetsTablePage
              key={`assets-${reloadKey}`}
              capabilities={capabilities}
              capabilitiesLoading={capabilitiesLoading}
              onConfigure={openSettings}
            />
          )}
        </TabPane>
        <TabPane tab={t('素材组')} itemKey='groups'>
          {channelEmpty || (
            <AssetGroupsPage
              key={`groups-${reloadKey}`}
              capabilities={capabilities}
              capabilitiesLoading={capabilitiesLoading}
              onConfigure={openSettings}
            />
          )}
        </TabPane>
      </Tabs>
    </div>
  );
};

export default Assets;
