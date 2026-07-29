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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Divider,
  Input,
  Modal,
  Select,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { Copy, Download, FileText } from 'lucide-react';
import { showError, showSuccess } from '../../../../helpers';
import {
  ASSET_BATCH_MAX,
  DEFAULT_ASSET_CAPABILITIES,
  createAsset,
  createAssetsBatch,
  downloadAssetTemplate,
  uploadAssetsExcel,
} from '../../../../services/assets';

const { Text } = Typography;

const UploadAssetsModal = ({
  visible,
  onCancel,
  onSuccess,
  groupOptions,
  groups,
  groupsLoading,
  capabilities,
  copyText,
  handleAssetError,
  t,
}) => {
  const caps = capabilities || DEFAULT_ASSET_CAPABILITIES;
  // 上游不支持批量创建时退化为单条
  const maxItems = caps.batchCreate ? caps.batchMaxItems || ASSET_BATCH_MAX : 1;

  const [groupId, setGroupId] = useState('');
  const [urlsText, setUrlsText] = useState('');
  const [name, setName] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [excelUploading, setExcelUploading] = useState(false);
  const [templateLoading, setTemplateLoading] = useState(false);
  // { mode: 'json' | 'excel', total, results: [{index, status, officialId, error}], urls }
  const [batchResult, setBatchResult] = useState(null);
  const fileInputRef = useRef(null);

  useEffect(() => {
    if (!visible) return;
    setUrlsText('');
    setName('');
    setBatchResult(null);
  }, [visible]);

  const urls = useMemo(
    () =>
      urlsText
        .split('\n')
        .map((line) => line.trim())
        .filter((line) => line.length > 0),
    [urlsText],
  );

  const selectedGroup = useMemo(
    () => (groups || []).find((group) => group.officialId === groupId),
    [groups, groupId],
  );

  const handleSubmit = async () => {
    if (!groupId) {
      showError(t('请先选择素材组'));
      return;
    }
    if (urls.length === 0) {
      showError(t('请至少填写一个素材 URL'));
      return;
    }
    if (urls.length > maxItems) {
      showError(
        caps.batchCreate
          ? t('单次最多上传 {{max}} 个素材', { max: maxItems })
          : t('当前上游不支持批量上传，请一次只填写一个 URL'),
      );
      return;
    }

    setSubmitting(true);
    try {
      if (urls.length === 1) {
        const payload = { groupId, url: urls[0] };
        if (name.trim()) payload.name = name.trim();
        await createAsset(payload);
        showSuccess(t('素材已提交，正在处理中'));
        setBatchResult(null);
        onSuccess?.();
        onCancel?.();
      } else {
        const data = await createAssetsBatch(
          urls.map((url) => ({ groupId, url })),
        );
        const results = data.results || [];
        const okCount = results.filter((item) => item.status === 'ok').length;
        setBatchResult({
          mode: 'json',
          total: data.total ?? urls.length,
          results,
          urls,
        });
        showSuccess(
          t('批量提交完成：成功 {{success}}/{{total}}', {
            success: okCount,
            total: results.length || urls.length,
          }),
        );
        onSuccess?.();
      }
    } catch (error) {
      handleAssetError?.(error);
    } finally {
      setSubmitting(false);
    }
  };

  const handleDownloadTemplate = async () => {
    setTemplateLoading(true);
    try {
      const data = await downloadAssetTemplate();
      const blob = new Blob([data], {
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = 'assets_batch_template.xlsx';
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } catch (error) {
      showError(t('模板下载失败，请稍后再试'));
    } finally {
      setTemplateLoading(false);
    }
  };

  const handleExcelChange = async (event) => {
    const file = event.target.files && event.target.files[0];
    // 允许连续选择同一个文件
    event.target.value = '';
    if (!file) return;

    setExcelUploading(true);
    try {
      const data = await uploadAssetsExcel(file);
      const results = data.results || [];
      const okCount = results.filter((item) => item.status === 'ok').length;
      setBatchResult({
        mode: 'excel',
        total: data.total ?? results.length,
        results,
      });
      showSuccess(
        t('批量提交完成：成功 {{success}}/{{total}}', {
          success: okCount,
          total: results.length,
        }),
      );
      onSuccess?.();
    } catch (error) {
      handleAssetError?.(error);
    } finally {
      setExcelUploading(false);
    }
  };

  const renderResultLabel = (item) => {
    if (batchResult?.mode === 'excel') {
      return t('第 {{index}} 行', { index: item.index });
    }
    const url = batchResult?.urls?.[item.index];
    return url || t('第 {{index}} 条', { index: item.index + 1 });
  };

  return (
    <Modal
      title={t('上传素材')}
      visible={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={submitting}
      okText={t('上传')}
      cancelText={t('取消')}
      width={620}
      maskClosable={false}
    >
      <div className='flex flex-col gap-3'>
        {/* 素材组 */}
        <div>
          <div className='mb-1'>
            <Text size='small'>
              {t('素材组')}
              <span className='text-semi-color-danger ml-1'>*</span>
            </Text>
          </div>
          <Select
            value={groupId}
            onChange={(value) => setGroupId(value)}
            optionList={groupOptions || []}
            loading={groupsLoading}
            placeholder={t('请选择素材组')}
            className='w-full'
            showClear
          />
        </div>

        {/* 素材 URL */}
        <div>
          <div className='mb-1'>
            <Text size='small'>
              {t('素材 URL')}
              <span className='text-semi-color-danger ml-1'>*</span>
            </Text>
          </div>
          <TextArea
            value={urlsText}
            onChange={(value) => setUrlsText(value)}
            autosize={{ minRows: 4, maxRows: 10 }}
            placeholder={t('每行一个 URL，单次最多 {{max}} 个', {
              max: maxItems,
            })}
          />
          <Text type='tertiary' size='small' className='block mt-1'>
            {t('已填写 {{total}} 个 URL', { total: urls.length })}
          </Text>
        </div>

        {/* 名称 */}
        <div>
          <div className='mb-1'>
            <Text size='small'>{t('名称')}</Text>
          </div>
          <Input
            value={name}
            onChange={(value) => setName(value)}
            placeholder={t('选填，仅在只填写一个 URL 时生效')}
            disabled={urls.length > 1}
            showClear
          />
        </div>

        {/* Excel 批量上传：仅在上游支持模板时展示 */}
        {caps.excelTemplate ? (
          <>
            <Divider margin='4px' />

            <div>
              <div className='flex items-center gap-2 mb-2'>
                <FileText size={14} />
                <Text size='small' strong>
                  {t('Excel 批量上传')}
                </Text>
              </div>

              <div className='flex flex-wrap items-center gap-2'>
                <Button
                  type='tertiary'
                  size='small'
                  icon={<Download size={14} />}
                  loading={templateLoading}
                  onClick={handleDownloadTemplate}
                >
                  {t('下载模板')}
                </Button>

                {selectedGroup ? (
                  <div className='flex items-center gap-1'>
                    <Text type='tertiary' size='small'>
                      {t('当前素材组 ID')}
                    </Text>
                    <Tag color='white' shape='circle'>
                      {selectedGroup.officialId}
                    </Tag>
                    <Button
                      type='tertiary'
                      theme='borderless'
                      size='small'
                      icon={<Copy size={14} />}
                      onClick={() => copyText?.(selectedGroup.officialId)}
                    />
                  </div>
                ) : (
                  <Text type='tertiary' size='small'>
                    {t('请先选择素材组以获取素材组 ID')}
                  </Text>
                )}

                <Button
                  type='primary'
                  size='small'
                  loading={excelUploading}
                  disabled={excelUploading}
                  onClick={() => fileInputRef.current?.click()}
                >
                  {t('选择 Excel 文件')}
                </Button>
                <input
                  ref={fileInputRef}
                  type='file'
                  accept='.xlsx'
                  className='hidden'
                  onChange={handleExcelChange}
                />
              </div>

              <Banner
                type='warning'
                closeIcon={null}
                className='!rounded-lg mt-2'
                description={t(
                  '服务端不会解析表格里的素材组，请先复制上方的素材组 ID，并手动填入表格的 groupId 列，否则素材会归入错误的素材组。',
                )}
              />
            </div>
          </>
        ) : null}

        {/* 批量结果 */}
        {batchResult ? (
          <div>
            <Divider margin='4px' />
            <Text size='small' strong className='block mb-2'>
              {t('批量结果（{{done}}/{{total}}）', {
                done: batchResult.results.length,
                total: batchResult.total,
              })}
            </Text>
            <div className='max-h-52 overflow-auto flex flex-col gap-1'>
              {batchResult.results.map((item) => (
                <div
                  key={`${item.index}-${item.officialId || 'err'}`}
                  className='flex items-center gap-2 text-xs'
                >
                  <Tag
                    color={item.status === 'ok' ? 'green' : 'red'}
                    shape='circle'
                    size='small'
                  >
                    {item.status === 'ok' ? t('成功') : t('失败')}
                  </Tag>
                  <Text
                    size='small'
                    ellipsis={{ showTooltip: true }}
                    style={{ maxWidth: 240 }}
                  >
                    {renderResultLabel(item)}
                  </Text>
                  <Text type='tertiary' size='small' ellipsis={{ showTooltip: true }}>
                    {item.status === 'ok' ? item.officialId : item.error}
                  </Text>
                </div>
              ))}
            </div>
          </div>
        ) : null}
      </div>
    </Modal>
  );
};

export default UploadAssetsModal;
