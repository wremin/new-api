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
import {
  Button,
  Image,
  Popconfirm,
  Space,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Copy,
  HelpCircle,
  Image as ImageIcon,
  Loader,
  Music,
  RefreshCw,
  Video,
} from 'lucide-react';
import { timestamp2string } from '../../../helpers';
import { ASSET_STATUS } from '../../../services/assets';

const { Text } = Typography;

// 空的名称/素材组：服务端异步回填，这里给一个弱化的占位
const renderPlaceholder = (t) => (
  <Text type='tertiary' size='small'>
    {t('同步中…')}
  </Text>
);

const renderPreview = (record, t) => {
  if (record.assetType === 'Image' && record.url) {
    return (
      <div className='w-10 h-10 rounded-md overflow-hidden flex items-center justify-center bg-semi-color-fill-0'>
        <Image src={record.url} width={40} height={40} alt={record.name || ''} />
      </div>
    );
  }

  let icon = <HelpCircle size={16} />;
  if (record.assetType === 'Video') icon = <Video size={16} />;
  if (record.assetType === 'Audio') icon = <Music size={16} />;
  if (record.assetType === 'Image') icon = <ImageIcon size={16} />;

  return (
    <Tooltip content={record.url || t('暂无预览')}>
      <div className='w-10 h-10 rounded-md flex items-center justify-center bg-semi-color-fill-0 text-semi-color-text-2'>
        {icon}
      </div>
    </Tooltip>
  );
};

const renderAssetType = (assetType, t) => {
  switch (assetType) {
    case 'Image':
      return (
        <Tag color='blue' shape='circle' prefixIcon={<ImageIcon size={14} />}>
          {t('图片')}
        </Tag>
      );
    case 'Video':
      return (
        <Tag color='violet' shape='circle' prefixIcon={<Video size={14} />}>
          {t('视频')}
        </Tag>
      );
    case 'Audio':
      return (
        <Tag color='cyan' shape='circle' prefixIcon={<Music size={14} />}>
          {t('音频')}
        </Tag>
      );
    default:
      return (
        <Tag color='grey' shape='circle' prefixIcon={<HelpCircle size={14} />}>
          {assetType || t('未知')}
        </Tag>
      );
  }
};

const renderRegion = (region, t) => {
  if (!region) return renderPlaceholder(t);
  if (region === 'intl') {
    return (
      <Tag color='orange' shape='circle'>
        {t('国际版 intl')}
      </Tag>
    );
  }
  if (region === 'cn') {
    return (
      <Tag color='green' shape='circle'>
        {t('国内版 cn')}
      </Tag>
    );
  }
  return (
    <Tag color='grey' shape='circle'>
      {region}
    </Tag>
  );
};

const renderStatus = (record, t) => {
  switch (record.status) {
    case ASSET_STATUS.PROCESSING:
      return (
        <Tag
          color='blue'
          shape='circle'
          prefixIcon={<Loader size={14} className='animate-spin' />}
        >
          {t('处理中')}
        </Tag>
      );
    case ASSET_STATUS.ACTIVE:
      return (
        <Tag color='green' shape='circle'>
          {t('可用')}
        </Tag>
      );
    case ASSET_STATUS.FAILED:
      return (
        <Tooltip content={record.failReason || t('上游未返回失败原因')}>
          <Tag color='red' shape='circle'>
            {t('失败')}
          </Tag>
        </Tooltip>
      );
    default:
      return (
        <Tag color='grey' shape='circle'>
          {record.status || t('未知')}
        </Tag>
      );
  }
};

export const getAssetsColumns = ({
  t,
  groupMap,
  filterByGroup,
  copyText,
  refreshAssetStatus,
  removeAsset,
  refreshingId,
}) => [
  {
    title: t('预览'),
    dataIndex: 'preview',
    key: 'preview',
    width: 80,
    render: (_, record) => renderPreview(record, t),
  },
  {
    title: t('名称'),
    dataIndex: 'name',
    key: 'name',
    render: (name) =>
      name ? (
        <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 200 }}>
          {name}
        </Text>
      ) : (
        renderPlaceholder(t)
      ),
  },
  {
    title: t('类型'),
    dataIndex: 'assetType',
    key: 'assetType',
    render: (assetType) => renderAssetType(assetType, t),
  },
  {
    title: t('素材组'),
    dataIndex: 'groupId',
    key: 'groupId',
    render: (groupId) => {
      if (!groupId) return renderPlaceholder(t);
      const group = groupMap?.[groupId];
      return (
        <Tag
          color='white'
          shape='circle'
          className='cursor-pointer'
          onClick={() => filterByGroup(groupId)}
        >
          {group?.name || groupId}
        </Tag>
      );
    },
  },
  {
    title: t('区域'),
    dataIndex: 'region',
    key: 'region',
    render: (region, record) => {
      const resolved = region || groupMap?.[record.groupId]?.region || '';
      return renderRegion(resolved, t);
    },
  },
  {
    title: t('状态'),
    dataIndex: 'status',
    key: 'status',
    render: (_, record) => renderStatus(record, t),
  },
  {
    title: t('引用'),
    dataIndex: 'assetRef',
    key: 'assetRef',
    render: (assetRef) =>
      assetRef ? (
        <Space spacing={4}>
          <Text
            code
            ellipsis={{ showTooltip: true }}
            style={{ maxWidth: 220 }}
          >
            {assetRef}
          </Text>
          <Button
            type='tertiary'
            theme='borderless'
            size='small'
            icon={<Copy size={14} />}
            onClick={() => copyText(assetRef)}
          />
        </Space>
      ) : (
        renderPlaceholder(t)
      ),
  },
  {
    title: t('创建时间'),
    dataIndex: 'createdAt',
    key: 'createdAt',
    render: (createdAt) =>
      createdAt ? (
        <Text size='small'>{timestamp2string(createdAt)}</Text>
      ) : (
        renderPlaceholder(t)
      ),
  },
  {
    title: t('操作'),
    dataIndex: 'operate',
    key: 'operate',
    fixed: 'right',
    render: (_, record) => (
      <Space spacing={4}>
        <Button
          type='tertiary'
          theme='borderless'
          size='small'
          icon={<RefreshCw size={14} />}
          loading={refreshingId === record.officialId}
          onClick={() => refreshAssetStatus(record)}
        >
          {t('刷新状态')}
        </Button>
        <Popconfirm
          title={t('确定删除该素材？')}
          content={t('删除后引用该素材的请求将会失败，且无法恢复。')}
          okText={t('删除')}
          cancelText={t('取消')}
          okType='danger'
          position='topRight'
          onConfirm={() => removeAsset(record)}
        >
          <Button type='danger' theme='borderless' size='small'>
            {t('删除')}
          </Button>
        </Popconfirm>
      </Space>
    ),
  },
];
