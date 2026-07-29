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
import { Button, Card, Empty, Typography } from '@douyinfe/semi-ui';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import { isRoot } from '../../../helpers';

const { Text } = Typography;

/**
 * 素材渠道未配置 / 存在多个可用渠道时的空状态。
 * 只有 root 能改这项配置（后端 RootAuth），因此只对 root 提供入口，
 * 直接在本页打开素材库设置；其余用户只看到说明文案。
 */
const AssetsChannelEmpty = ({ channelError, onConfigure, t }) => {
  const isAmbiguous = channelError?.code === 'assets_channel_ambiguous';
  const description = isAmbiguous
    ? t('检测到多个可用的素材渠道，请管理员在运营设置中指定唯一的素材渠道。')
    : t('管理员尚未在运营设置中配置素材渠道，素材库暂时不可用。');

  return (
    <Card className='!rounded-2xl' bordered>
      <Empty
        image={<IllustrationConstruction style={{ width: 150, height: 150 }} />}
        darkModeImage={
          <IllustrationConstructionDark style={{ width: 150, height: 150 }} />
        }
        title={t('素材渠道未配置')}
        description={description}
        style={{ padding: 30 }}
      >
        {channelError?.message ? (
          <Text type='tertiary' size='small' className='block mt-2'>
            {channelError.message}
          </Text>
        ) : null}
        {/*
          只有 root 能改素材渠道配置（后端是 RootAuth），因此只对 root 给操作入口。
          非 root 管理员既没有权限，运营设置页也没有对应字段，给链接只会把人带到死路上。
        */}
        {isRoot() && onConfigure ? (
          <Button
            type='primary'
            theme='solid'
            size='small'
            className='mt-4'
            onClick={onConfigure}
          >
            {t('配置素材渠道')}
          </Button>
        ) : null}
      </Empty>
    </Card>
  );
};

export default AssetsChannelEmpty;
