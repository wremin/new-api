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

import React, { useMemo } from 'react';
import { Button, Empty, Space, Tag, Typography } from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Copy } from 'lucide-react';
import CardTable from '../../common/ui/CardTable';
import { timestamp2string } from '../../../helpers';

const { Text } = Typography;

const renderRegionTag = (region, t) => {
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
      {region || t('未知')}
    </Tag>
  );
};

const AssetGroupsTable = ({ groups, loading, compactMode, copyText, t }) => {
  const columns = useMemo(
    () => [
      {
        title: t('名称'),
        dataIndex: 'name',
        key: 'name',
        render: (name, record) => (
          <Space spacing={4}>
            <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 180 }}>
              {name || record.officialId}
            </Text>
            <Button
              type='tertiary'
              theme='borderless'
              size='small'
              icon={<Copy size={14} />}
              onClick={() => copyText?.(record.officialId)}
            />
          </Space>
        ),
      },
      {
        title: t('描述'),
        dataIndex: 'description',
        key: 'description',
        render: (description) =>
          description ? (
            <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 260 }}>
              {description}
            </Text>
          ) : (
            <Text type='tertiary' size='small'>
              {t('无')}
            </Text>
          ),
      },
      {
        title: t('区域'),
        dataIndex: 'region',
        key: 'region',
        render: (region) => renderRegionTag(region, t),
      },
      {
        title: t('素材数量'),
        dataIndex: '_count',
        key: 'count',
        render: (count) => (
          <Tag color='blue' shape='circle'>
            {count?.assets ?? 0}
          </Tag>
        ),
      },
      {
        title: t('创建时间'),
        dataIndex: 'createdAt',
        key: 'createdAt',
        render: (createdAt) => {
          if (!createdAt) {
            return (
              <Text type='tertiary' size='small'>
                {t('未知')}
              </Text>
            );
          }
          // createdAt 为 UNIX 秒；若上游返回字符串则原样展示
          const text =
            typeof createdAt === 'number'
              ? timestamp2string(createdAt)
              : `${createdAt}`;
          return <Text size='small'>{text}</Text>;
        },
      },
    ],
    [t, copyText],
  );

  return (
    <CardTable
      columns={columns}
      dataSource={groups}
      rowKey='officialId'
      loading={loading}
      scroll={compactMode ? undefined : { x: 'max-content' }}
      className='rounded-xl overflow-hidden'
      size='middle'
      empty={
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={t('暂无素材组')}
          style={{ padding: 30 }}
        />
      }
      hidePagination={true}
    />
  );
};

export default AssetGroupsTable;
