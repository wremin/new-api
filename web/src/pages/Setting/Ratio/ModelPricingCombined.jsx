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

import React, { useEffect, useState } from 'react';
import { Button, Card, Input, Radio, RadioGroup, Space } from '@douyinfe/semi-ui';
import { IconSave } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import ModelPricingEditor from './components/ModelPricingEditor';
import ModelRatioSettings from './ModelRatioSettings';
import { API, showError, showSuccess } from '../../../helpers';

const DEFAULT_LONG_CONTEXT_THRESHOLD = 272 * 1024;

function parseThreshold(value) {
  if (value === '' || value === null || value === undefined) {
    return DEFAULT_LONG_CONTEXT_THRESHOLD;
  }
  const num = Number(String(value).trim());
  return Number.isFinite(num) && num >= 0 ? Math.floor(num) : DEFAULT_LONG_CONTEXT_THRESHOLD;
}

export default function ModelPricingCombined({ options, refresh }) {
  const { t } = useTranslation();
  const [editMode, setEditMode] = useState('visual');
  const [threshold, setThreshold] = useState(String(DEFAULT_LONG_CONTEXT_THRESHOLD));
  const [savingThreshold, setSavingThreshold] = useState(false);

  useEffect(() => {
    const rawValue = options?.LongContextThreshold;
    const parsed = parseThreshold(rawValue);
    setThreshold(String(parsed));
  }, [options?.LongContextThreshold]);

  const handleThresholdChange = (value) => {
    // 仅允许非负整数
    const normalized = value.replace(/[^0-9]/g, '');
    setThreshold(normalized);
  };

  const handleSaveThreshold = async () => {
    const value = parseThreshold(threshold);
    setSavingThreshold(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'LongContextThreshold',
        value: String(value),
      });
      if (res?.data?.success) {
        showSuccess(t('保存成功'));
        await refresh();
      } else {
        throw new Error(res?.data?.message || t('保存失败'));
      }
    } catch (error) {
      showError(error.message || t('保存失败'));
    } finally {
      setSavingThreshold(false);
    }
  };

  return (
    <div>
      <Card
        bodyStyle={{ padding: 16 }}
        style={{ marginBottom: 16, background: 'var(--semi-color-fill-0)' }}
      >
        <div className='font-medium mb-2'>{t('长文本定价阈值')}</div>
        <div className='text-xs text-gray-500 mb-3'>
          {t(
            '当一次请求的实际总 tokens（prompt + completion）超过该阈值时，将使用下方配置的「长文本价格」计费。默认 272k tokens。',
          )}
        </div>
        <Space>
          <Input
            value={threshold}
            onChange={handleThresholdChange}
            suffix='tokens'
            style={{ width: 220 }}
          />
          <Button
            type='primary'
            icon={<IconSave />}
            loading={savingThreshold}
            onClick={handleSaveThreshold}
          >
            {t('保存阈值')}
          </Button>
        </Space>
      </Card>

      <div style={{ marginTop: 12, marginBottom: 16 }}>
        <RadioGroup
          type='button'
          size='small'
          value={editMode}
          onChange={(e) => setEditMode(e.target.value)}
        >
          <Radio value='visual'>{t('可视化编辑')}</Radio>
          <Radio value='manual'>{t('手动编辑')}</Radio>
        </RadioGroup>
      </div>
      {editMode === 'visual' ? (
        <ModelPricingEditor options={options} refresh={refresh} />
      ) : (
        <ModelRatioSettings options={options} refresh={refresh} />
      )}
    </div>
  );
}
