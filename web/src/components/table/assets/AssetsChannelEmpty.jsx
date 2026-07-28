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
import { Link } from 'react-router-dom';
import { Button, Card, Empty, Typography } from '@douyinfe/semi-ui';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import { isAdmin } from '../../../helpers';

const { Text } = Typography;

/**
 * 素材渠道未配置 / 存在多个可用渠道时的空状态。
 * 普通用户只提示联系管理员，管理员额外展示前往运营设置的入口。
 */
const AssetsChannelEmpty = ({ channelError, t }) => {
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
        {isAdmin() ? (
          <Link to='/console/setting' className='inline-block mt-4'>
            <Button type='primary' theme='solid' size='small'>
              {t('前往运营设置')}
            </Button>
          </Link>
        ) : null}
      </Empty>
    </Card>
  );
};

export default AssetsChannelEmpty;
