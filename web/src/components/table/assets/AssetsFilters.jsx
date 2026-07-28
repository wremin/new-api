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
import { Button, Form } from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import { RefreshCw } from 'lucide-react';

const AssetsFilters = ({
  formInitValues,
  setFormApi,
  formApi,
  refresh,
  refreshWithSync,
  groupOptions,
  groupsLoading,
  loading,
  t,
}) => {
  return (
    <Form
      initValues={formInitValues}
      getFormApi={(api) => setFormApi(api)}
      onSubmit={refresh}
      allowEmpty={true}
      autoComplete='off'
      layout='vertical'
      trigger='change'
      stopValidateWithError={false}
    >
      <div className='flex flex-col gap-2'>
        <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2'>
          {/* 素材组 */}
          <Form.Select
            field='groupId'
            placeholder={t('全部素材组')}
            optionList={[
              { label: t('全部素材组'), value: '' },
              ...(groupOptions || []),
            ]}
            loading={groupsLoading}
            className='w-full'
            showClear
            pure
            size='small'
          />

          {/* 状态 */}
          <Form.Select
            field='status'
            placeholder={t('全部状态')}
            optionList={[
              { label: t('全部状态'), value: '' },
              { label: t('处理中'), value: 'Processing' },
              { label: t('可用'), value: 'Active' },
              { label: t('失败'), value: 'Failed' },
            ]}
            className='w-full'
            showClear
            pure
            size='small'
          />

          {/* 类型 */}
          <Form.Select
            field='assetType'
            placeholder={t('全部类型')}
            optionList={[
              { label: t('全部类型'), value: '' },
              { label: t('图片'), value: 'Image' },
              { label: t('视频'), value: 'Video' },
              { label: t('音频'), value: 'Audio' },
            ]}
            className='w-full'
            showClear
            pure
            size='small'
          />

          {/* 关键词 */}
          <Form.Input
            field='keyword'
            prefix={<IconSearch />}
            placeholder={t('搜索素材名称')}
            showClear
            pure
            size='small'
          />
        </div>

        {/* 操作按钮区域 */}
        <div className='flex justify-between items-center'>
          <div></div>
          <div className='flex gap-2'>
            <Button
              type='tertiary'
              htmlType='submit'
              loading={loading}
              size='small'
            >
              {t('查询')}
            </Button>
            <Button
              type='tertiary'
              onClick={() => {
                if (formApi) {
                  formApi.reset();
                  // 重置后立即查询，使用setTimeout确保表单重置完成
                  setTimeout(() => {
                    refresh();
                  }, 100);
                }
              }}
              size='small'
            >
              {t('重置')}
            </Button>
            <Button
              type='tertiary'
              icon={<RefreshCw size={14} />}
              onClick={refreshWithSync}
              loading={loading}
              size='small'
            >
              {t('刷新')}
            </Button>
          </div>
        </div>
      </div>
    </Form>
  );
};

export default AssetsFilters;
