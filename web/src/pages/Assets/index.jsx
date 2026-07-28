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
import { Tabs, TabPane } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import AssetsTablePage from '../../components/table/assets';
import AssetGroupsPage from '../../components/table/assets/AssetGroupsPage';

const Assets = () => {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState('assets');

  return (
    <div className='mt-[60px] px-2'>
      <Tabs
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key)}
        type='card'
      >
        <TabPane tab={t('素材列表')} itemKey='assets'>
          <AssetsTablePage />
        </TabPane>
        <TabPane tab={t('素材组')} itemKey='groups'>
          <AssetGroupsPage />
        </TabPane>
      </Tabs>
    </div>
  );
};

export default Assets;
