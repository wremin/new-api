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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Input,
  Modal,
  Radio,
  RadioGroup,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { showError } from '../../../../helpers';
import { DEFAULT_ASSET_CAPABILITIES } from '../../../../services/assets';

const { Text } = Typography;

const CreateAssetGroupModal = ({
  visible,
  onCancel,
  onSubmit,
  creating,
  capabilities,
  t,
}) => {
  const caps = capabilities || DEFAULT_ASSET_CAPABILITIES;
  // 有区域的上游（seegen）用 region，无区域的上游（stelloria）用 groupType，二者互斥
  const groupTypes = useMemo(() => caps.groupTypes || [], [caps.groupTypes]);
  const defaultGroupType = groupTypes[0] || '';

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [region, setRegion] = useState('cn');
  const [groupType, setGroupType] = useState(defaultGroupType);

  useEffect(() => {
    if (!visible) return;
    setName('');
    setDescription('');
    setRegion('cn');
    setGroupType(defaultGroupType);
  }, [visible, defaultGroupType]);

  const handleOk = async () => {
    if (!name.trim()) {
      showError(t('请输入素材组名称'));
      return;
    }
    const payload = { name: name.trim() };
    // 只能带上上游支持的那一个字段，否则后端会返回 asset_unsupported_by_provider
    if (caps.regions) {
      payload.region = region;
    } else if (groupType) {
      payload.groupType = groupType;
    }
    if (description.trim()) payload.description = description.trim();
    await onSubmit?.(payload);
  };

  return (
    <Modal
      title={t('新建素材组')}
      visible={visible}
      onCancel={onCancel}
      onOk={handleOk}
      confirmLoading={creating}
      okText={t('创建')}
      cancelText={t('取消')}
      maskClosable={false}
    >
      <div className='flex flex-col gap-3'>
        {/* 名称 */}
        <div>
          <div className='mb-1'>
            <Text size='small'>
              {t('名称')}
              <span className='text-semi-color-danger ml-1'>*</span>
            </Text>
          </div>
          <Input
            value={name}
            onChange={(value) => setName(value)}
            placeholder={t('请输入素材组名称')}
            showClear
          />
        </div>

        {/* 描述 */}
        <div>
          <div className='mb-1'>
            <Text size='small'>{t('描述')}</Text>
          </div>
          <TextArea
            value={description}
            onChange={(value) => setDescription(value)}
            autosize={{ minRows: 2, maxRows: 5 }}
            placeholder={t('选填，用于说明该素材组的用途')}
          />
        </div>

        {/* 区域（仅支持区域的上游） */}
        {caps.regions ? (
          <div>
            <div className='mb-1'>
              <Text size='small'>
                {t('区域')}
                <span className='text-semi-color-danger ml-1'>*</span>
              </Text>
            </div>
            <RadioGroup
              type='button'
              value={region}
              onChange={(e) => setRegion(e.target.value)}
            >
              <Radio value='cn'>{t('国内版 cn')}</Radio>
              <Radio value='intl'>{t('国际版 intl')}</Radio>
            </RadioGroup>
            <Banner
              type='danger'
              closeIcon={null}
              className='!rounded-lg mt-2'
              description={t(
                '区域一经创建不可修改，请谨慎选择：使用国际版或大尺度模型时必须选择 intl，否则该素材组下的素材将无法使用。',
              )}
            />
          </div>
        ) : null}

        {/* 素材组类型（无区域但有类型的上游） */}
        {!caps.regions && groupTypes.length > 0 ? (
          <div>
            <div className='mb-1'>
              <Text size='small'>
                {t('素材组类型')}
                <span className='text-semi-color-danger ml-1'>*</span>
              </Text>
            </div>
            <RadioGroup
              type='button'
              value={groupType}
              onChange={(e) => setGroupType(e.target.value)}
            >
              {groupTypes.map((item) => (
                <Radio key={item} value={item}>
                  {item}
                </Radio>
              ))}
            </RadioGroup>
            <Banner
              type='danger'
              closeIcon={null}
              className='!rounded-lg mt-2'
              description={t(
                '素材组类型一经创建不可修改，请谨慎选择：该类型决定素材组可用于哪些模型能力。',
              )}
            />
          </div>
        ) : null}
      </div>
    </Modal>
  );
};

export default CreateAssetGroupModal;
