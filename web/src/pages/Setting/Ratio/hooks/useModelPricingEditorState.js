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

import { useEffect, useMemo, useState } from 'react';
import { API, showError, showSuccess } from '../../../../helpers';

export const PAGE_SIZE = 10;
export const getPriceSuffix = (priceMode = 'USD') =>
  priceMode === 'CNY' ? '¥/1M tokens' : '$/1M tokens';
const EMPTY_CANDIDATE_MODEL_NAMES = [];

const getPriceMode = () => {
  if (typeof window === 'undefined') return 'USD';
  return localStorage.getItem('quota_display_type') || 'USD';
};

const getUsdExchangeRate = () => {
  if (typeof window === 'undefined') return 7.3;
  try {
    const statusStr = localStorage.getItem('status');
    if (statusStr) {
      const s = JSON.parse(statusStr);
      return s?.usd_exchange_rate || 7.3;
    }
  } catch (e) {}
  return 7.3;
};

const EMPTY_MODEL = {
  name: '',
  billingMode: 'per-token',
  fixedPrice: '',
  inputPrice: '',
  completionPrice: '',
  lockedCompletionRatio: '',
  completionRatioLocked: false,
  cachePrice: '',
  createCachePrice: '',
  imagePrice: '',
  audioInputPrice: '',
  audioOutputPrice: '',
  longContextInputPrice: '',
  longContextCompletionPrice: '',
  longContextCachePrice: '',
  longContextCreateCachePrice: '',
  rawRatios: {
    modelRatio: '',
    completionRatio: '',
    cacheRatio: '',
    createCacheRatio: '',
    imageRatio: '',
    audioRatio: '',
    audioCompletionRatio: '',
    longContextModelRatio: '',
    longContextCompletionRatio: '',
    longContextCacheRatio: '',
    longContextCreateCacheRatio: '',
  },
  hasConflict: false,
};

const NUMERIC_INPUT_REGEX = /^(\d+(\.\d*)?|\.\d*)?$/;

export const hasValue = (value) =>
  value !== '' && value !== null && value !== undefined && value !== false;

const toNumericString = (value) => {
  if (!hasValue(value) && value !== 0) {
    return '';
  }
  const num = Number(value);
  return Number.isFinite(num) ? String(num) : '';
};

const toNumberOrNull = (value) => {
  if (!hasValue(value) && value !== 0) {
    return null;
  }
  const num = Number(value);
  return Number.isFinite(num) ? num : null;
};

const formatNumber = (value) => {
  const num = toNumberOrNull(value);
  if (num === null) {
    return '';
  }
  return parseFloat(num.toFixed(12)).toString();
};

const toNormalizedNumber = (value) => {
  const formatted = formatNumber(value);
  return formatted === '' ? null : Number(formatted);
};

const parseOptionJSON = (rawValue) => {
  if (!rawValue || rawValue.trim() === '') {
    return {};
  }
  try {
    const parsed = JSON.parse(rawValue);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (error) {
    console.error('JSON解析错误:', error);
    return {};
  }
};

const ratioToDisplayPrice = (ratio, priceMode, usdExchangeRate) => {
  const num = toNumberOrNull(ratio);
  if (num === null) return '';
  const usdPrice = num * 2;
  if (priceMode === 'CNY') {
    return formatNumber(usdPrice * usdExchangeRate);
  }
  return formatNumber(usdPrice);
};

const normalizeCompletionRatioMeta = (rawMeta) => {
  if (!rawMeta || typeof rawMeta !== 'object' || Array.isArray(rawMeta)) {
    return {
      locked: false,
      ratio: '',
    };
  }

  return {
    locked: Boolean(rawMeta.locked),
    ratio: toNumericString(rawMeta.ratio),
  };
};

const buildModelState = (name, sourceMaps, priceMode, usdExchangeRate) => {
  const modelRatio = toNumericString(sourceMaps.ModelRatio[name]);
  const completionRatio = toNumericString(sourceMaps.CompletionRatio[name]);
  const completionRatioMeta = normalizeCompletionRatioMeta(
    sourceMaps.CompletionRatioMeta?.[name],
  );
  const cacheRatio = toNumericString(sourceMaps.CacheRatio[name]);
  const createCacheRatio = toNumericString(sourceMaps.CreateCacheRatio[name]);
  const imageRatio = toNumericString(sourceMaps.ImageRatio[name]);
  const audioRatio = toNumericString(sourceMaps.AudioRatio[name]);
  const audioCompletionRatio = toNumericString(
    sourceMaps.AudioCompletionRatio[name],
  );
  const longContextModelRatio = toNumericString(
    sourceMaps.LongContextModelRatio?.[name],
  );
  const longContextCompletionRatio = toNumericString(
    sourceMaps.LongContextCompletionRatio?.[name],
  );
  const longContextCacheRatio = toNumericString(
    sourceMaps.LongContextCacheRatio?.[name],
  );
  const longContextCreateCacheRatio = toNumericString(
    sourceMaps.LongContextCreateCacheRatio?.[name],
  );
  let fixedPrice = toNumericString(sourceMaps.ModelPrice[name]);
  if (priceMode === 'CNY' && hasValue(fixedPrice)) {
    fixedPrice = formatNumber(Number(fixedPrice) * usdExchangeRate);
  }
  const inputPrice = ratioToDisplayPrice(
    modelRatio,
    priceMode,
    usdExchangeRate,
  );
  const inputPriceNumber = toNumberOrNull(inputPrice);
  const audioInputPrice =
    inputPriceNumber !== null && hasValue(audioRatio)
      ? formatNumber(inputPriceNumber * Number(audioRatio))
      : '';
  const longContextInputPrice = ratioToDisplayPrice(
    longContextModelRatio,
    priceMode,
    usdExchangeRate,
  );
  const longContextInputPriceNumber = toNumberOrNull(longContextInputPrice);

  return {
    ...EMPTY_MODEL,
    name,
    billingMode: hasValue(fixedPrice) ? 'per-request' : 'per-token',
    fixedPrice,
    inputPrice,
    completionRatioLocked: completionRatioMeta.locked,
    lockedCompletionRatio: completionRatioMeta.ratio,
    completionPrice:
      inputPriceNumber !== null && hasValue(completionRatio)
        ? formatNumber(inputPriceNumber * Number(completionRatio))
        : '',
    cachePrice:
      inputPriceNumber !== null && hasValue(cacheRatio)
        ? formatNumber(inputPriceNumber * Number(cacheRatio))
        : '',
    createCachePrice:
      inputPriceNumber !== null && hasValue(createCacheRatio)
        ? formatNumber(inputPriceNumber * Number(createCacheRatio))
        : '',
    imagePrice:
      inputPriceNumber !== null && hasValue(imageRatio)
        ? formatNumber(inputPriceNumber * Number(imageRatio))
        : '',
    audioInputPrice,
    audioOutputPrice:
      toNumberOrNull(audioInputPrice) !== null && hasValue(audioCompletionRatio)
        ? formatNumber(Number(audioInputPrice) * Number(audioCompletionRatio))
        : '',
    longContextInputPrice,
    longContextCompletionPrice:
      longContextInputPriceNumber !== null &&
      hasValue(longContextCompletionRatio)
        ? formatNumber(
            longContextInputPriceNumber * Number(longContextCompletionRatio),
          )
        : '',
    longContextCachePrice:
      longContextInputPriceNumber !== null && hasValue(longContextCacheRatio)
        ? formatNumber(
            longContextInputPriceNumber * Number(longContextCacheRatio),
          )
        : '',
    longContextCreateCachePrice:
      longContextInputPriceNumber !== null &&
      hasValue(longContextCreateCacheRatio)
        ? formatNumber(
            longContextInputPriceNumber * Number(longContextCreateCacheRatio),
          )
        : '',
    rawRatios: {
      modelRatio,
      completionRatio,
      cacheRatio,
      createCacheRatio,
      imageRatio,
      audioRatio,
      audioCompletionRatio,
      longContextModelRatio,
      longContextCompletionRatio,
      longContextCacheRatio,
      longContextCreateCacheRatio,
    },
    hasConflict:
      hasValue(fixedPrice) &&
      [
        modelRatio,
        completionRatio,
        cacheRatio,
        createCacheRatio,
        imageRatio,
        audioRatio,
        audioCompletionRatio,
        longContextModelRatio,
        longContextCompletionRatio,
        longContextCacheRatio,
        longContextCreateCacheRatio,
      ].some(hasValue),
  };
};

export const isBasePricingUnset = (model) =>
  !hasValue(model.fixedPrice) && !hasValue(model.inputPrice);

export const getModelWarnings = (model, t) => {
  if (!model) {
    return [];
  }
  const warnings = [];
  const hasDerivedPricing = [
    model.inputPrice,
    model.completionPrice,
    model.cachePrice,
    model.createCachePrice,
    model.imagePrice,
    model.audioInputPrice,
    model.audioOutputPrice,
    model.longContextInputPrice,
    model.longContextCompletionPrice,
    model.longContextCachePrice,
    model.longContextCreateCachePrice,
  ].some(hasValue);

  if (model.hasConflict) {
    warnings.push(
      t('当前模型同时存在按次价格和倍率配置，保存时会按当前计费方式覆盖。'),
    );
  }

  if (
    !hasValue(model.inputPrice) &&
    [
      model.rawRatios.completionRatio,
      model.rawRatios.cacheRatio,
      model.rawRatios.createCacheRatio,
      model.rawRatios.imageRatio,
      model.rawRatios.audioRatio,
      model.rawRatios.audioCompletionRatio,
    ].some(hasValue)
  ) {
    warnings.push(
      t(
        '当前模型存在未显式设置输入倍率的扩展倍率；填写输入价格后会自动换算为价格字段。',
      ),
    );
  }

  if (
    !hasValue(model.longContextInputPrice) &&
    [
      model.rawRatios.longContextCompletionRatio,
      model.rawRatios.longContextCacheRatio,
      model.rawRatios.longContextCreateCacheRatio,
    ].some(hasValue)
  ) {
    warnings.push(
      t(
        '当前模型存在未显式设置长文本输入倍率的长文本扩展倍率；填写长文本输入价格后会自动换算为价格字段。',
      ),
    );
  }

  if (
    model.billingMode === 'per-token' &&
    hasDerivedPricing &&
    !hasValue(model.inputPrice)
  ) {
    warnings.push(t('按量计费下需要先填写输入价格，才能保存其它价格项。'));
  }

  if (
    model.billingMode === 'per-token' &&
    hasValue(model.audioOutputPrice) &&
    !hasValue(model.audioInputPrice)
  ) {
    warnings.push(t('填写音频补全价格前，需要先填写音频输入价格。'));
  }

  return warnings;
};

export const buildSummaryText = (model, t, priceMode = 'USD') => {
  const symbol = priceMode === 'CNY' ? '¥' : '$';
  if (model.billingMode === 'per-request' && hasValue(model.fixedPrice)) {
    return `${t('按次')} ${symbol}${model.fixedPrice} / ${t('次')}`;
  }

  if (hasValue(model.inputPrice)) {
    const extraCount = [
      model.completionPrice,
      model.cachePrice,
      model.createCachePrice,
      model.imagePrice,
      model.audioInputPrice,
      model.audioOutputPrice,
    ].filter(hasValue).length;
    const extraLabel =
      extraCount > 0 ? `，${t('额外价格项')} ${extraCount}` : '';
    const hasLongContext = [
      model.longContextInputPrice,
      model.longContextCompletionPrice,
      model.longContextCachePrice,
      model.longContextCreateCachePrice,
    ].some(hasValue);
    const longContextLabel = hasLongContext ? `，${t('已配置长文本价格')}` : '';
    return `${t('输入')} ${symbol}${model.inputPrice}${extraLabel}${longContextLabel}`;
  }

  return t('未设置价格');
};

export const buildOptionalFieldToggles = (model) => ({
  completionPrice: hasValue(model.completionPrice),
  cachePrice: hasValue(model.cachePrice),
  createCachePrice: hasValue(model.createCachePrice),
  imagePrice: hasValue(model.imagePrice),
  audioInputPrice: hasValue(model.audioInputPrice),
  audioOutputPrice: hasValue(model.audioOutputPrice),
  longContextInputPrice: hasValue(model.longContextInputPrice),
  longContextCompletionPrice: hasValue(model.longContextCompletionPrice),
  longContextCachePrice: hasValue(model.longContextCachePrice),
  longContextCreateCachePrice: hasValue(model.longContextCreateCachePrice),
});

const serializeModel = (model, t, priceMode, usdExchangeRate) => {
  const result = {
    ModelPrice: null,
    ModelRatio: null,
    CompletionRatio: null,
    CacheRatio: null,
    CreateCacheRatio: null,
    ImageRatio: null,
    AudioRatio: null,
    AudioCompletionRatio: null,
    LongContextModelRatio: null,
    LongContextCompletionRatio: null,
    LongContextCacheRatio: null,
    LongContextCreateCacheRatio: null,
  };

  if (model.billingMode === 'per-request') {
    if (hasValue(model.fixedPrice)) {
      const fixedPriceNumber = toNumberOrNull(model.fixedPrice);
      result.ModelPrice = toNormalizedNumber(
        priceMode === 'CNY'
          ? fixedPriceNumber / usdExchangeRate
          : fixedPriceNumber,
      );
    }
    return result;
  }

  const inputPrice = toNumberOrNull(model.inputPrice);
  const completionPrice = toNumberOrNull(model.completionPrice);
  const cachePrice = toNumberOrNull(model.cachePrice);
  const createCachePrice = toNumberOrNull(model.createCachePrice);
  const imagePrice = toNumberOrNull(model.imagePrice);
  const audioInputPrice = toNumberOrNull(model.audioInputPrice);
  const audioOutputPrice = toNumberOrNull(model.audioOutputPrice);
  const longContextInputPrice = toNumberOrNull(model.longContextInputPrice);
  const longContextCompletionPrice = toNumberOrNull(
    model.longContextCompletionPrice,
  );
  const longContextCachePrice = toNumberOrNull(model.longContextCachePrice);
  const longContextCreateCachePrice = toNumberOrNull(
    model.longContextCreateCachePrice,
  );

  const hasDependentPrice = [
    completionPrice,
    cachePrice,
    createCachePrice,
    imagePrice,
    audioInputPrice,
    audioOutputPrice,
    longContextCompletionPrice,
    longContextCachePrice,
    longContextCreateCachePrice,
  ].some((value) => value !== null);

  if (inputPrice === null) {
    if (hasDependentPrice) {
      throw new Error(
        t(
          '模型 {{name}} 缺少输入价格，无法计算补全/缓存/图片/音频价格对应的倍率',
          {
            name: model.name,
          },
        ),
      );
    }

    if (hasValue(model.rawRatios.modelRatio)) {
      result.ModelRatio = toNormalizedNumber(model.rawRatios.modelRatio);
    }
    if (hasValue(model.rawRatios.completionRatio)) {
      result.CompletionRatio = toNormalizedNumber(
        model.rawRatios.completionRatio,
      );
    }
    if (hasValue(model.rawRatios.cacheRatio)) {
      result.CacheRatio = toNormalizedNumber(model.rawRatios.cacheRatio);
    }
    if (hasValue(model.rawRatios.createCacheRatio)) {
      result.CreateCacheRatio = toNormalizedNumber(
        model.rawRatios.createCacheRatio,
      );
    }
    if (hasValue(model.rawRatios.imageRatio)) {
      result.ImageRatio = toNormalizedNumber(model.rawRatios.imageRatio);
    }
    if (hasValue(model.rawRatios.audioRatio)) {
      result.AudioRatio = toNormalizedNumber(model.rawRatios.audioRatio);
    }
    if (hasValue(model.rawRatios.audioCompletionRatio)) {
      result.AudioCompletionRatio = toNormalizedNumber(
        model.rawRatios.audioCompletionRatio,
      );
    }
    if (hasValue(model.rawRatios.longContextModelRatio)) {
      result.LongContextModelRatio = toNormalizedNumber(
        model.rawRatios.longContextModelRatio,
      );
    }
    if (hasValue(model.rawRatios.longContextCompletionRatio)) {
      result.LongContextCompletionRatio = toNormalizedNumber(
        model.rawRatios.longContextCompletionRatio,
      );
    }
    if (hasValue(model.rawRatios.longContextCacheRatio)) {
      result.LongContextCacheRatio = toNormalizedNumber(
        model.rawRatios.longContextCacheRatio,
      );
    }
    if (hasValue(model.rawRatios.longContextCreateCacheRatio)) {
      result.LongContextCreateCacheRatio = toNormalizedNumber(
        model.rawRatios.longContextCreateCacheRatio,
      );
    }
    return result;
  }

  result.ModelRatio = toNormalizedNumber(
    priceMode === 'CNY'
      ? inputPrice / (2 * usdExchangeRate)
      : inputPrice / 2,
  );

  if (completionPrice !== null) {
    result.CompletionRatio = toNormalizedNumber(completionPrice / inputPrice);
  }
  if (cachePrice !== null) {
    result.CacheRatio = toNormalizedNumber(cachePrice / inputPrice);
  }
  if (createCachePrice !== null) {
    result.CreateCacheRatio = toNormalizedNumber(createCachePrice / inputPrice);
  }
  if (imagePrice !== null) {
    result.ImageRatio = toNormalizedNumber(imagePrice / inputPrice);
  }
  if (audioInputPrice !== null) {
    result.AudioRatio = toNormalizedNumber(audioInputPrice / inputPrice);
  }
  if (audioOutputPrice !== null) {
    if (audioInputPrice === null || audioInputPrice === 0) {
      throw new Error(
        t('模型 {{name}} 缺少音频输入价格，无法计算音频补全倍率', {
          name: model.name,
        }),
      );
    }
    result.AudioCompletionRatio = toNormalizedNumber(
      audioOutputPrice / audioInputPrice,
    );
  }

  // 长文本价格序列化：长文本输入价格为独立基础，不依赖普通输入价格
  if (longContextInputPrice !== null) {
    result.LongContextModelRatio = toNormalizedNumber(
      priceMode === 'CNY'
        ? longContextInputPrice / (2 * usdExchangeRate)
        : longContextInputPrice / 2,
    );

    if (longContextCompletionPrice !== null) {
      result.LongContextCompletionRatio = toNormalizedNumber(
        longContextCompletionPrice / longContextInputPrice,
      );
    }
    if (longContextCachePrice !== null) {
      result.LongContextCacheRatio = toNormalizedNumber(
        longContextCachePrice / longContextInputPrice,
      );
    }
    if (longContextCreateCachePrice !== null) {
      result.LongContextCreateCacheRatio = toNormalizedNumber(
        longContextCreateCachePrice / longContextInputPrice,
      );
    }
  } else if (
    longContextCompletionPrice !== null ||
    longContextCachePrice !== null ||
    longContextCreateCachePrice !== null
  ) {
    throw new Error(
      t(
        '模型 {{name}} 缺少长文本输入价格，无法计算长文本补全/缓存价格对应的倍率',
        {
          name: model.name,
        },
      ),
    );
  }

  return result;
};

export const buildPreviewRows = (
  model,
  t,
  priceMode = 'USD',
  usdExchangeRate = 7.3,
) => {
  if (!model) return [];

  const displayToUsdFactor = priceMode === 'CNY' ? usdExchangeRate : 1;

  if (model.billingMode === 'per-request') {
    return [
      {
        key: 'ModelPrice',
        label: 'ModelPrice',
        value: hasValue(model.fixedPrice)
          ? formatNumber(
              toNumberOrNull(model.fixedPrice) / displayToUsdFactor,
            )
          : t('空'),
      },
    ];
  }

  const inputPrice = toNumberOrNull(model.inputPrice);
  if (inputPrice === null) {
    return [
      {
        key: 'ModelRatio',
        label: 'ModelRatio',
        value: hasValue(model.rawRatios.modelRatio)
          ? model.rawRatios.modelRatio
          : t('空'),
      },
      {
        key: 'CompletionRatio',
        label: 'CompletionRatio',
        value: hasValue(model.rawRatios.completionRatio)
          ? model.rawRatios.completionRatio
          : t('空'),
      },
      {
        key: 'CacheRatio',
        label: 'CacheRatio',
        value: hasValue(model.rawRatios.cacheRatio)
          ? model.rawRatios.cacheRatio
          : t('空'),
      },
      {
        key: 'CreateCacheRatio',
        label: 'CreateCacheRatio',
        value: hasValue(model.rawRatios.createCacheRatio)
          ? model.rawRatios.createCacheRatio
          : t('空'),
      },
      {
        key: 'ImageRatio',
        label: 'ImageRatio',
        value: hasValue(model.rawRatios.imageRatio)
          ? model.rawRatios.imageRatio
          : t('空'),
      },
      {
        key: 'AudioRatio',
        label: 'AudioRatio',
        value: hasValue(model.rawRatios.audioRatio)
          ? model.rawRatios.audioRatio
          : t('空'),
      },
      {
        key: 'AudioCompletionRatio',
        label: 'AudioCompletionRatio',
        value: hasValue(model.rawRatios.audioCompletionRatio)
          ? model.rawRatios.audioCompletionRatio
          : t('空'),
      },
      {
        key: 'LongContextModelRatio',
        label: 'LongContextModelRatio',
        value: hasValue(model.rawRatios.longContextModelRatio)
          ? model.rawRatios.longContextModelRatio
          : t('空'),
      },
      {
        key: 'LongContextCompletionRatio',
        label: 'LongContextCompletionRatio',
        value: hasValue(model.rawRatios.longContextCompletionRatio)
          ? model.rawRatios.longContextCompletionRatio
          : t('空'),
      },
      {
        key: 'LongContextCacheRatio',
        label: 'LongContextCacheRatio',
        value: hasValue(model.rawRatios.longContextCacheRatio)
          ? model.rawRatios.longContextCacheRatio
          : t('空'),
      },
      {
        key: 'LongContextCreateCacheRatio',
        label: 'LongContextCreateCacheRatio',
        value: hasValue(model.rawRatios.longContextCreateCacheRatio)
          ? model.rawRatios.longContextCreateCacheRatio
          : t('空'),
      },
    ];
  }

  const completionPrice = toNumberOrNull(model.completionPrice);
  const cachePrice = toNumberOrNull(model.cachePrice);
  const createCachePrice = toNumberOrNull(model.createCachePrice);
  const imagePrice = toNumberOrNull(model.imagePrice);
  const audioInputPrice = toNumberOrNull(model.audioInputPrice);
  const audioOutputPrice = toNumberOrNull(model.audioOutputPrice);

  return [
    {
      key: 'ModelRatio',
      label: 'ModelRatio',
      value: formatNumber(inputPrice / 2 / displayToUsdFactor),
    },
    {
      key: 'CompletionRatio',
      label: 'CompletionRatio',
      value:
        completionPrice !== null
          ? formatNumber(completionPrice / inputPrice)
          : t('空'),
    },
    {
      key: 'CacheRatio',
      label: 'CacheRatio',
      value:
        cachePrice !== null ? formatNumber(cachePrice / inputPrice) : t('空'),
    },
    {
      key: 'CreateCacheRatio',
      label: 'CreateCacheRatio',
      value:
        createCachePrice !== null
          ? formatNumber(createCachePrice / inputPrice)
          : t('空'),
    },
    {
      key: 'ImageRatio',
      label: 'ImageRatio',
      value:
        imagePrice !== null ? formatNumber(imagePrice / inputPrice) : t('空'),
    },
    {
      key: 'AudioRatio',
      label: 'AudioRatio',
      value:
        audioInputPrice !== null
          ? formatNumber(audioInputPrice / inputPrice)
          : t('空'),
    },
    {
      key: 'AudioCompletionRatio',
      label: 'AudioCompletionRatio',
      value:
        audioOutputPrice !== null &&
        audioInputPrice !== null &&
        audioInputPrice !== 0
          ? formatNumber(audioOutputPrice / audioInputPrice)
          : t('空'),
    },
    {
      key: 'LongContextModelRatio',
      label: 'LongContextModelRatio',
      value: hasValue(model.rawRatios.longContextModelRatio)
        ? model.rawRatios.longContextModelRatio
        : t('空'),
    },
    {
      key: 'LongContextCompletionRatio',
      label: 'LongContextCompletionRatio',
      value: hasValue(model.rawRatios.longContextCompletionRatio)
        ? model.rawRatios.longContextCompletionRatio
        : t('空'),
    },
    {
      key: 'LongContextCacheRatio',
      label: 'LongContextCacheRatio',
      value: hasValue(model.rawRatios.longContextCacheRatio)
        ? model.rawRatios.longContextCacheRatio
        : t('空'),
    },
    {
      key: 'LongContextCreateCacheRatio',
      label: 'LongContextCreateCacheRatio',
      value: hasValue(model.rawRatios.longContextCreateCacheRatio)
        ? model.rawRatios.longContextCreateCacheRatio
        : t('空'),
    },
  ];
};

export function useModelPricingEditorState({
  options,
  refresh,
  t,
  candidateModelNames = EMPTY_CANDIDATE_MODEL_NAMES,
  filterMode = 'all',
}) {
  const [models, setModels] = useState([]);
  const [initialVisibleModelNames, setInitialVisibleModelNames] = useState([]);
  const [selectedModelName, setSelectedModelName] = useState('');
  const [selectedModelNames, setSelectedModelNames] = useState([]);
  const [searchText, setSearchText] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [conflictOnly, setConflictOnly] = useState(false);
  const [optionalFieldToggles, setOptionalFieldToggles] = useState({});
  const [priceMode, setPriceMode] = useState(() => getPriceMode());
  const [usdExchangeRate, setUsdExchangeRate] = useState(() =>
    getUsdExchangeRate(),
  );

  useEffect(() => {
    setPriceMode(getPriceMode());
    setUsdExchangeRate(getUsdExchangeRate());
  }, [options]);

  useEffect(() => {
    const sourceMaps = {
      ModelPrice: parseOptionJSON(options.ModelPrice),
      ModelRatio: parseOptionJSON(options.ModelRatio),
      CompletionRatio: parseOptionJSON(options.CompletionRatio),
      CompletionRatioMeta: parseOptionJSON(options.CompletionRatioMeta),
      CacheRatio: parseOptionJSON(options.CacheRatio),
      CreateCacheRatio: parseOptionJSON(options.CreateCacheRatio),
      ImageRatio: parseOptionJSON(options.ImageRatio),
      AudioRatio: parseOptionJSON(options.AudioRatio),
      AudioCompletionRatio: parseOptionJSON(options.AudioCompletionRatio),
      LongContextModelRatio: parseOptionJSON(options.LongContextModelRatio),
      LongContextCompletionRatio: parseOptionJSON(
        options.LongContextCompletionRatio,
      ),
      LongContextCacheRatio: parseOptionJSON(options.LongContextCacheRatio),
      LongContextCreateCacheRatio: parseOptionJSON(
        options.LongContextCreateCacheRatio,
      ),
    };

    const names = new Set([
      ...candidateModelNames,
      ...Object.keys(sourceMaps.ModelPrice),
      ...Object.keys(sourceMaps.ModelRatio),
      ...Object.keys(sourceMaps.CompletionRatio),
      ...Object.keys(sourceMaps.CompletionRatioMeta),
      ...Object.keys(sourceMaps.CacheRatio),
      ...Object.keys(sourceMaps.CreateCacheRatio),
      ...Object.keys(sourceMaps.ImageRatio),
      ...Object.keys(sourceMaps.AudioRatio),
      ...Object.keys(sourceMaps.AudioCompletionRatio),
      ...Object.keys(sourceMaps.LongContextModelRatio),
      ...Object.keys(sourceMaps.LongContextCompletionRatio),
      ...Object.keys(sourceMaps.LongContextCacheRatio),
      ...Object.keys(sourceMaps.LongContextCreateCacheRatio),
    ]);

    const nextModels = Array.from(names)
      .map((name) =>
        buildModelState(name, sourceMaps, priceMode, usdExchangeRate),
      )
      .sort((a, b) => a.name.localeCompare(b.name));

    setModels(nextModels);
    setInitialVisibleModelNames(
      filterMode === 'unset'
        ? nextModels
            .filter((model) => isBasePricingUnset(model))
            .map((model) => model.name)
        : nextModels.map((model) => model.name),
    );
    setOptionalFieldToggles(
      nextModels.reduce((acc, model) => {
        acc[model.name] = buildOptionalFieldToggles(model);
        return acc;
      }, {}),
    );
    setSelectedModelName((previous) => {
      if (previous && nextModels.some((model) => model.name === previous)) {
        return previous;
      }
      const nextVisibleModels =
        filterMode === 'unset'
          ? nextModels.filter((model) => isBasePricingUnset(model))
          : nextModels;
      return nextVisibleModels[0]?.name || '';
    });
  }, [candidateModelNames, filterMode, options]);

  const visibleModels = useMemo(() => {
    return filterMode === 'unset'
      ? models.filter((model) => initialVisibleModelNames.includes(model.name))
      : models;
  }, [filterMode, initialVisibleModelNames, models]);

  const filteredModels = useMemo(() => {
    return visibleModels.filter((model) => {
      const keyword = searchText.trim().toLowerCase();
      const keywordMatch = keyword
        ? model.name.toLowerCase().includes(keyword)
        : true;
      const conflictMatch = conflictOnly ? model.hasConflict : true;
      return keywordMatch && conflictMatch;
    });
  }, [conflictOnly, searchText, visibleModels]);

  const pagedData = useMemo(() => {
    const start = (currentPage - 1) * PAGE_SIZE;
    return filteredModels.slice(start, start + PAGE_SIZE);
  }, [currentPage, filteredModels]);

  const selectedModel = useMemo(
    () =>
      visibleModels.find((model) => model.name === selectedModelName) || null,
    [selectedModelName, visibleModels],
  );

  const selectedWarnings = useMemo(
    () => getModelWarnings(selectedModel, t),
    [selectedModel, t],
  );

  const previewRows = useMemo(
    () => buildPreviewRows(selectedModel, t, priceMode, usdExchangeRate),
    [selectedModel, t, priceMode, usdExchangeRate],
  );

  useEffect(() => {
    setCurrentPage(1);
  }, [searchText, conflictOnly, filterMode, candidateModelNames]);

  useEffect(() => {
    setSelectedModelNames((previous) =>
      previous.filter((name) =>
        visibleModels.some((model) => model.name === name),
      ),
    );
  }, [visibleModels]);

  useEffect(() => {
    if (visibleModels.length === 0) {
      setSelectedModelName('');
      return;
    }
    if (!visibleModels.some((model) => model.name === selectedModelName)) {
      setSelectedModelName(visibleModels[0].name);
    }
  }, [selectedModelName, visibleModels]);

  const upsertModel = (name, updater) => {
    setModels((previous) =>
      previous.map((model) => {
        if (model.name !== name) return model;
        return typeof updater === 'function' ? updater(model) : updater;
      }),
    );
  };

  const isOptionalFieldEnabled = (model, field) => {
    if (!model) return false;
    const modelToggles = optionalFieldToggles[model.name];
    if (modelToggles && typeof modelToggles[field] === 'boolean') {
      return modelToggles[field];
    }
    return buildOptionalFieldToggles(model)[field];
  };

  const updateOptionalFieldToggle = (modelName, field, checked) => {
    setOptionalFieldToggles((prev) => ({
      ...prev,
      [modelName]: {
        ...(prev[modelName] || {}),
        [field]: checked,
      },
    }));
  };

  const handleOptionalFieldToggle = (field, checked) => {
    if (!selectedModel) return;

    updateOptionalFieldToggle(selectedModel.name, field, checked);

    if (checked) {
      return;
    }

    upsertModel(selectedModel.name, (model) => {
      const nextModel = { ...model, [field]: '' };

      if (field === 'audioInputPrice') {
        nextModel.audioOutputPrice = '';
        setOptionalFieldToggles((prev) => ({
          ...prev,
          [selectedModel.name]: {
            ...(prev[selectedModel.name] || {}),
            audioInputPrice: false,
            audioOutputPrice: false,
          },
        }));
      }

      return nextModel;
    });
  };

  const fillDerivedPricesFromBase = (model, nextInputPrice) => {
    const baseNumber = toNumberOrNull(nextInputPrice);
    if (baseNumber === null) {
      return model;
    }

    return {
      ...model,
      completionPrice:
        !hasValue(model.completionPrice) &&
        hasValue(model.rawRatios.completionRatio)
          ? formatNumber(baseNumber * Number(model.rawRatios.completionRatio))
          : model.completionPrice,
      cachePrice:
        !hasValue(model.cachePrice) && hasValue(model.rawRatios.cacheRatio)
          ? formatNumber(baseNumber * Number(model.rawRatios.cacheRatio))
          : model.cachePrice,
      createCachePrice:
        !hasValue(model.createCachePrice) &&
        hasValue(model.rawRatios.createCacheRatio)
          ? formatNumber(baseNumber * Number(model.rawRatios.createCacheRatio))
          : model.createCachePrice,
      imagePrice:
        !hasValue(model.imagePrice) && hasValue(model.rawRatios.imageRatio)
          ? formatNumber(baseNumber * Number(model.rawRatios.imageRatio))
          : model.imagePrice,
      audioInputPrice:
        !hasValue(model.audioInputPrice) && hasValue(model.rawRatios.audioRatio)
          ? formatNumber(baseNumber * Number(model.rawRatios.audioRatio))
          : model.audioInputPrice,
      audioOutputPrice:
        !hasValue(model.audioOutputPrice) &&
        hasValue(model.rawRatios.audioRatio) &&
        hasValue(model.rawRatios.audioCompletionRatio)
          ? formatNumber(
              baseNumber *
                Number(model.rawRatios.audioRatio) *
                Number(model.rawRatios.audioCompletionRatio),
            )
          : model.audioOutputPrice,
    };
  };

  const fillLongContextDerivedPricesFromBase = (
    model,
    nextLongContextInputPrice,
  ) => {
    const baseNumber = toNumberOrNull(nextLongContextInputPrice);
    if (baseNumber === null) {
      return model;
    }

    return {
      ...model,
      longContextCompletionPrice:
        !hasValue(model.longContextCompletionPrice) &&
        hasValue(model.rawRatios.longContextCompletionRatio)
          ? formatNumber(
              baseNumber * Number(model.rawRatios.longContextCompletionRatio),
            )
          : model.longContextCompletionPrice,
      longContextCachePrice:
        !hasValue(model.longContextCachePrice) &&
        hasValue(model.rawRatios.longContextCacheRatio)
          ? formatNumber(
              baseNumber * Number(model.rawRatios.longContextCacheRatio),
            )
          : model.longContextCachePrice,
      longContextCreateCachePrice:
        !hasValue(model.longContextCreateCachePrice) &&
        hasValue(model.rawRatios.longContextCreateCacheRatio)
          ? formatNumber(
              baseNumber * Number(model.rawRatios.longContextCreateCacheRatio),
            )
          : model.longContextCreateCachePrice,
    };
  };

  const handleNumericFieldChange = (field, value) => {
    if (!selectedModel || !NUMERIC_INPUT_REGEX.test(value)) {
      return;
    }

    upsertModel(selectedModel.name, (model) => {
      const updatedModel = { ...model, [field]: value };

      if (field === 'inputPrice') {
        return fillDerivedPricesFromBase(updatedModel, value);
      }

      if (field === 'longContextInputPrice') {
        return fillLongContextDerivedPricesFromBase(updatedModel, value);
      }

      return updatedModel;
    });
  };

  const handleBillingModeChange = (value) => {
    if (!selectedModel) return;
    upsertModel(selectedModel.name, (model) => ({
      ...model,
      billingMode: value,
    }));
  };

  const addModel = (modelName) => {
    const trimmedName = modelName.trim();
    if (!trimmedName) {
      showError(t('请输入模型名称'));
      return false;
    }
    if (models.some((model) => model.name === trimmedName)) {
      showError(t('模型名称已存在'));
      return false;
    }

    const nextModel = {
      ...EMPTY_MODEL,
      name: trimmedName,
      rawRatios: { ...EMPTY_MODEL.rawRatios },
    };

    setModels((previous) => [nextModel, ...previous]);
    setOptionalFieldToggles((prev) => ({
      ...prev,
      [trimmedName]: buildOptionalFieldToggles(nextModel),
    }));
    setSelectedModelName(trimmedName);
    setCurrentPage(1);
    return true;
  };

  const deleteModel = (name) => {
    const nextModels = models.filter((model) => model.name !== name);
    setModels(nextModels);
    setOptionalFieldToggles((prev) => {
      const next = { ...prev };
      delete next[name];
      return next;
    });
    setSelectedModelNames((previous) =>
      previous.filter((item) => item !== name),
    );
    if (selectedModelName === name) {
      setSelectedModelName(nextModels[0]?.name || '');
    }
  };

  const applySelectedModelPricing = () => {
    if (!selectedModel) {
      showError(t('请先选择一个作为模板的模型'));
      return false;
    }
    if (selectedModelNames.length === 0) {
      showError(t('请先勾选需要批量设置的模型'));
      return false;
    }

    const sourceToggles = optionalFieldToggles[selectedModel.name] || {};

    setModels((previous) =>
      previous.map((model) => {
        if (!selectedModelNames.includes(model.name)) {
          return model;
        }

        const nextModel = {
          ...model,
          billingMode: selectedModel.billingMode,
          fixedPrice: selectedModel.fixedPrice,
          inputPrice: selectedModel.inputPrice,
          completionPrice: selectedModel.completionPrice,
          cachePrice: selectedModel.cachePrice,
          createCachePrice: selectedModel.createCachePrice,
          imagePrice: selectedModel.imagePrice,
          audioInputPrice: selectedModel.audioInputPrice,
          audioOutputPrice: selectedModel.audioOutputPrice,
          longContextInputPrice: selectedModel.longContextInputPrice,
          longContextCompletionPrice: selectedModel.longContextCompletionPrice,
          longContextCachePrice: selectedModel.longContextCachePrice,
          longContextCreateCachePrice:
            selectedModel.longContextCreateCachePrice,
        };

        return nextModel;
      }),
    );

    setOptionalFieldToggles((previous) => {
      const next = { ...previous };
      selectedModelNames.forEach((modelName) => {
        next[modelName] = {
          completionPrice: Boolean(sourceToggles.completionPrice),
          cachePrice: Boolean(sourceToggles.cachePrice),
          createCachePrice: Boolean(sourceToggles.createCachePrice),
          imagePrice: Boolean(sourceToggles.imagePrice),
          audioInputPrice: Boolean(sourceToggles.audioInputPrice),
          audioOutputPrice:
            Boolean(sourceToggles.audioInputPrice) &&
            Boolean(sourceToggles.audioOutputPrice),
          longContextInputPrice: Boolean(sourceToggles.longContextInputPrice),
          longContextCompletionPrice: Boolean(
            sourceToggles.longContextCompletionPrice,
          ),
          longContextCachePrice: Boolean(sourceToggles.longContextCachePrice),
          longContextCreateCachePrice: Boolean(
            sourceToggles.longContextCreateCachePrice,
          ),
        };
      });
      return next;
    });

    showSuccess(
      t('已将模型 {{name}} 的价格配置批量应用到 {{count}} 个模型', {
        name: selectedModel.name,
        count: selectedModelNames.length,
      }),
    );
    return true;
  };

  const handleSubmit = async () => {
    setLoading(true);
    try {
      const output = {
        ModelPrice: {},
        ModelRatio: {},
        CompletionRatio: {},
        CacheRatio: {},
        CreateCacheRatio: {},
        ImageRatio: {},
        AudioRatio: {},
        AudioCompletionRatio: {},
        LongContextModelRatio: {},
        LongContextCompletionRatio: {},
        LongContextCacheRatio: {},
        LongContextCreateCacheRatio: {},
      };

      for (const model of models) {
        const serialized = serializeModel(
          model,
          t,
          priceMode,
          usdExchangeRate,
        );
        Object.entries(serialized).forEach(([key, value]) => {
          if (value !== null) {
            output[key][model.name] = value;
          }
        });
      }

      const requestQueue = Object.entries(output).map(([key, value]) =>
        API.put('/api/option/', {
          key,
          value: JSON.stringify(value, null, 2),
        }),
      );

      const results = await Promise.all(requestQueue);
      for (const res of results) {
        if (!res?.data?.success) {
          throw new Error(res?.data?.message || t('保存失败，请重试'));
        }
      }

      showSuccess(t('保存成功'));
      await refresh();
    } catch (error) {
      console.error('保存失败:', error);
      showError(error.message || t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  return {
    models,
    selectedModel,
    selectedModelName,
    selectedModelNames,
    setSelectedModelName,
    setSelectedModelNames,
    searchText,
    setSearchText,
    currentPage,
    setCurrentPage,
    loading,
    conflictOnly,
    setConflictOnly,
    filteredModels,
    pagedData,
    selectedWarnings,
    previewRows,
    isOptionalFieldEnabled,
    handleOptionalFieldToggle,
    handleNumericFieldChange,
    handleBillingModeChange,
    handleSubmit,
    addModel,
    deleteModel,
    applySelectedModelPricing,
    priceMode,
    currencySymbol: priceMode === 'CNY' ? '¥' : '$',
    usdExchangeRate,
  };
}
