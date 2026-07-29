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

import { API } from '../helpers';

/**
 * 素材库相关接口封装
 *
 * 注意：/v1/assets 系列接口的错误格式为 { error: { message, type, code } } +
 * 非 2xx 状态码，与 /api/* 的 { success, message, data } 完全不同，
 * 因此所有请求都带上 skipErrorHandler，由调用方自行处理错误。
 */

export const ASSET_STATUS = {
  PROCESSING: 'Processing',
  ACTIVE: 'Active',
  FAILED: 'Failed',
};

export const ASSET_TYPES = ['Image', 'Video', 'Audio'];

export const ASSET_REGIONS = ['cn', 'intl'];

// 渠道未配置 / 存在多个可用渠道时的错误码，需要展示空状态而非报错
export const ASSET_CHANNEL_ERROR_CODES = [
  'assets_channel_not_configured',
  'assets_channel_ambiguous',
];

export const ASSET_RATE_LIMIT_CODE = 'assets_rate_limit_exceeded';

// 单次批量上传的最大条数（上游能力未知时的兜底值）
export const ASSET_BATCH_MAX = 50;

/**
 * 上游能力的默认值：按 seegen（功能最全的上游）取值，
 * 这样在 /v1/assets/capabilities 返回前不会先闪现出「功能被禁用」的界面。
 */
export const DEFAULT_ASSET_CAPABILITIES = {
  provider: '',
  batchCreate: true,
  excelTemplate: true,
  regions: true,
  groupTypes: [],
  renameAsset: false,
  deleteGroup: false,
  batchMaxItems: ASSET_BATCH_MAX,
};

const baseConfig = { skipErrorHandler: true };

/**
 * 将 /v1/assets 的错误响应归一化为 { code, type, message }
 * @param {*} error axios 错误对象
 */
export function parseAssetError(error) {
  const payload = error?.response?.data;
  const detail =
    payload && typeof payload === 'object' && !(payload instanceof Blob)
      ? payload.error
      : null;

  if (detail && typeof detail === 'object') {
    return {
      code: detail.code || '',
      type: detail.type || '',
      message: detail.message || '',
    };
  }

  return {
    code: '',
    type: '',
    message: error?.message || String(error || ''),
  };
}

/**
 * 是否为「渠道未配置 / 渠道不唯一」错误
 */
export function isAssetChannelError(parsedError) {
  return ASSET_CHANNEL_ERROR_CODES.includes(parsedError?.code);
}

/**
 * 将 /v1/assets/capabilities 的响应归一化，缺失字段回退到 seegen 默认值
 */
export function normalizeAssetCapabilities(payload) {
  const data = payload && typeof payload === 'object' ? payload : {};
  const groupTypes = Array.isArray(data.groupTypes)
    ? data.groupTypes.filter((item) => typeof item === 'string' && item)
    : [];
  const batchMaxItems = Number(data.batchMaxItems);

  return {
    provider: typeof data.provider === 'string' ? data.provider : '',
    // 布尔能力缺省视为支持，避免上游只返回部分字段时误禁用功能
    batchCreate: data.batchCreate !== false,
    excelTemplate: data.excelTemplate !== false,
    regions: data.regions !== false,
    groupTypes,
    renameAsset: data.renameAsset === true,
    deleteGroup: data.deleteGroup === true,
    batchMaxItems:
      Number.isFinite(batchMaxItems) && batchMaxItems > 0
        ? Math.floor(batchMaxItems)
        : ASSET_BATCH_MAX,
  };
}

export async function getAssetCapabilities() {
  const res = await API.get('/v1/assets/capabilities', {
    ...baseConfig,
    disableDuplicate: true,
  });
  return normalizeAssetCapabilities(res.data);
}

export async function fetchAssets(params = {}) {
  const query = {};
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return;
    query[key] = value;
  });
  const res = await API.get('/v1/assets', {
    ...baseConfig,
    params: query,
    disableDuplicate: true,
  });
  return res.data || {};
}

export async function fetchAssetGroups() {
  const res = await API.get('/v1/assets/groups', {
    ...baseConfig,
    disableDuplicate: true,
  });
  return Array.isArray(res.data) ? res.data : [];
}

export async function createAssetGroup(payload) {
  const res = await API.post('/v1/assets/groups', payload, baseConfig);
  return res.data;
}

export async function createAsset(payload) {
  const res = await API.post('/v1/assets', payload, baseConfig);
  return res.data;
}

export async function createAssetsBatch(items) {
  const res = await API.post('/v1/assets/batch', items, baseConfig);
  return res.data || {};
}

export async function uploadAssetsExcel(file) {
  const formData = new FormData();
  formData.append('file', file);
  // 不手动设置 Content-Type，交由浏览器/axios 自动附带 multipart 的 boundary
  const res = await API.post('/v1/assets/batch', formData, baseConfig);
  return res.data || {};
}

export async function downloadAssetTemplate() {
  const res = await API.get('/v1/assets/batch/template', {
    ...baseConfig,
    responseType: 'blob',
    disableDuplicate: true,
  });
  return res.data;
}

export async function refreshAsset(officialId) {
  const res = await API.get(`/v1/assets/${officialId}`, {
    ...baseConfig,
    disableDuplicate: true,
  });
  return res.data;
}

export async function deleteAsset(officialId) {
  const res = await API.delete(`/v1/assets/${officialId}`, baseConfig);
  return res.data;
}
