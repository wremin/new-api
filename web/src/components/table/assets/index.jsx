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
import AssetsTable from './AssetsTable';
import AssetsActions from './AssetsActions';
import AssetsFilters from './AssetsFilters';
import AssetsChannelEmpty from './AssetsChannelEmpty';
import UploadAssetsModal from './modals/UploadAssetsModal';
import { useAssetsData } from '../../../hooks/assets/useAssetsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const AssetsPage = ({
  capabilities,
  capabilitiesLoading = false,
  onConfigure,
}) => {
  const assetsData = useAssetsData(capabilities);
  const isMobile = useIsMobile();

  if (assetsData.channelError) {
    return (
      <AssetsChannelEmpty
        channelError={assetsData.channelError}
        onConfigure={onConfigure}
        t={assetsData.t}
      />
    );
  }

  // 能力未就绪前不展示依赖能力的控件，统一走表格的 loading 态
  const pageData = { ...assetsData, capabilitiesLoading };

  return (
    <>
      {/* Modals */}
      <UploadAssetsModal
        visible={assetsData.showUploadModal}
        onCancel={() => assetsData.setShowUploadModal(false)}
        onSuccess={() => assetsData.loadAssets(1, assetsData.pageSize)}
        groups={assetsData.groups}
        groupOptions={assetsData.groupOptions}
        groupsLoading={assetsData.groupsLoading}
        capabilities={assetsData.capabilities}
        copyText={assetsData.copyText}
        handleAssetError={assetsData.handleAssetError}
        t={assetsData.t}
      />

      <Layout>
        <CardPro
          type='type2'
          statsArea={<AssetsActions {...pageData} />}
          searchArea={<AssetsFilters {...assetsData} />}
          paginationArea={createCardProPagination({
            currentPage: assetsData.activePage,
            pageSize: assetsData.pageSize,
            total: assetsData.assetCount,
            onPageChange: assetsData.handlePageChange,
            onPageSizeChange: assetsData.handlePageSizeChange,
            isMobile: isMobile,
            t: assetsData.t,
          })}
          t={assetsData.t}
        >
          <AssetsTable {...pageData} />
        </CardPro>
      </Layout>
    </>
  );
};

export default AssetsPage;
