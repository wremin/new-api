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

import React from 'react';
import { Layout } from '@douyinfe/semi-ui';
import CardPro from '../../common/ui/CardPro';
import AssetGroupsTable from './AssetGroupsTable';
import AssetGroupsActions from './AssetGroupsActions';
import AssetsChannelEmpty from './AssetsChannelEmpty';
import CreateAssetGroupModal from './modals/CreateAssetGroupModal';
import { useAssetGroupsData } from '../../../hooks/assets/useAssetGroupsData';

const AssetGroupsPage = ({
  capabilities,
  capabilitiesLoading = false,
  onConfigure,
}) => {
  const groupsData = useAssetGroupsData(capabilities);

  if (groupsData.channelError) {
    return (
      <AssetsChannelEmpty
        channelError={groupsData.channelError}
        onConfigure={onConfigure}
        t={groupsData.t}
      />
    );
  }

  // 能力未就绪前不展示依赖能力的控件，统一走表格的 loading 态
  const pageData = { ...groupsData, capabilitiesLoading };

  return (
    <>
      {/* Modals */}
      <CreateAssetGroupModal
        visible={groupsData.showCreateModal}
        onCancel={() => groupsData.setShowCreateModal(false)}
        onSubmit={groupsData.createGroup}
        creating={groupsData.creating}
        capabilities={groupsData.capabilities}
        t={groupsData.t}
      />

      <Layout>
        <CardPro
          type='type2'
          statsArea={<AssetGroupsActions {...pageData} />}
          t={groupsData.t}
        >
          <AssetGroupsTable {...pageData} />
        </CardPro>
      </Layout>
    </>
  );
};

export default AssetGroupsPage;
