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
import { Button, Popconfirm } from '@douyinfe/semi-ui';
import { IconDelete } from '@douyinfe/semi-icons';

const UsersActions = ({
  setShowAddUser,
  selectedRowKeys,
  onBatchDelete,
  t,
}) => {
  // Add new user
  const handleAddUser = () => {
    setShowAddUser(true);
  };

  const hasSelection = selectedRowKeys && selectedRowKeys.length > 0;

  return (
    <div className='flex gap-2 w-full md:w-auto order-2 md:order-1'>
      <Button className='w-full md:w-auto' onClick={handleAddUser} size='small'>
        {t('添加用户')}
      </Button>
      {hasSelection && (
        <Popconfirm
          title={t('确认批量删除')}
          content={t(
            '确定要删除选中的 {{count}} 个用户吗？此操作为软删除（禁用），可在之后恢复。',
            {
              count: selectedRowKeys.length,
            },
          )}
          onConfirm={onBatchDelete}
          position='bottom'
        >
          <Button
            type='danger'
            theme='light'
            size='small'
            icon={<IconDelete size='small' />}
          >
            {t('批量删除')} ({selectedRowKeys.length})
          </Button>
        </Popconfirm>
      )}
    </div>
  );
};

export default UsersActions;
