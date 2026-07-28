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

// 单次批量上传的最大条数（与后端保持一致）
export const ASSET_BATCH_MAX = 50;

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
