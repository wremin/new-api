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

import React, { useEffect, useState, useRef } from 'react';
import {
  Banner,
  Button,
  Form,
  Row,
  Col,
  Typography,
  Spin,
} from '@douyinfe/semi-ui';
const { Text } = Typography;
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsPaymentGatewayWechatPay(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    WechatPayEnabled: false,
    WechatPayAppID: '',
    WechatPayMchID: '',
    WechatPayKey: '',
    WechatPayNotifyURL: '',
    WechatPayReturnURL: '',
    WechatPayMinTopUp: 1,
  });
  const [originInputs, setOriginInputs] = useState({});
  const formApiRef = useRef(null);

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        WechatPayEnabled:
          props.options.WechatPayEnabled !== undefined
            ? props.options.WechatPayEnabled
            : false,
        WechatPayAppID: props.options.WechatPayAppID || '',
        WechatPayMchID: props.options.WechatPayMchID || '',
        WechatPayKey: props.options.WechatPayKey || '',
        WechatPayNotifyURL: props.options.WechatPayNotifyURL || '',
        WechatPayReturnURL: props.options.WechatPayReturnURL || '',
        WechatPayMinTopUp:
          props.options.WechatPayMinTopUp !== undefined
            ? parseFloat(props.options.WechatPayMinTopUp)
            : 1,
      };
      setInputs(currentInputs);
      setOriginInputs({ ...currentInputs });
      formApiRef.current.setValues(currentInputs);
    }
  }, [props.options]);

  const handleFormChange = (values) => {
    setInputs(values);
  };

  const submitWechatPaySetting = async () => {
    if (props.options.ServerAddress === '') {
      showError(t('请先填写服务器地址'));
      return;
    }

    setLoading(true);
    try {
      const options = [];

      if (
        originInputs['WechatPayEnabled'] !== inputs.WechatPayEnabled &&
        inputs.WechatPayEnabled !== undefined
      ) {
        options.push({
          key: 'WechatPayEnabled',
          value: inputs.WechatPayEnabled ? 'true' : 'false',
        });
      }
      if (inputs.WechatPayAppID !== '') {
        options.push({ key: 'WechatPayAppID', value: inputs.WechatPayAppID });
      }
      if (inputs.WechatPayMchID !== '') {
        options.push({ key: 'WechatPayMchID', value: inputs.WechatPayMchID });
      }
      if (inputs.WechatPayKey !== '') {
        options.push({ key: 'WechatPayKey', value: inputs.WechatPayKey });
      }
      if (inputs.WechatPayNotifyURL !== '') {
        options.push({ key: 'WechatPayNotifyURL', value: inputs.WechatPayNotifyURL });
      }
      if (inputs.WechatPayReturnURL !== '') {
        options.push({ key: 'WechatPayReturnURL', value: inputs.WechatPayReturnURL });
      }
      if (
        inputs.WechatPayMinTopUp !== undefined &&
        inputs.WechatPayMinTopUp !== null
      ) {
        options.push({
          key: 'WechatPayMinTopUp',
          value: inputs.WechatPayMinTopUp.toString(),
        });
      }

      // 发送请求
      const requestQueue = options.map((opt) =>
        API.put('/api/option/', {
          key: opt.key,
          value: opt.value,
        }),
      );

      const results = await Promise.all(requestQueue);

      // 检查所有请求是否成功
      const errorResults = results.filter((res) => !res.data.success);
      if (errorResults.length > 0) {
        errorResults.forEach((res) => {
          showError(res.data.message);
        });
      } else {
        showSuccess(t('更新成功'));
        // 更新本地存储的原始值
        setOriginInputs({ ...inputs });
        props.refresh?.();
      }
    } catch (error) {
      showError(t('更新失败'));
    }
    setLoading(false);
  };

  return (
    <Spin spinning={loading}>
      <Form
        initValues={inputs}
        onValueChange={handleFormChange}
        getFormApi={(api) => (formApiRef.current = api)}
      >
        <Form.Section text={t('微信支付设置')}>
          <Text>
            {t('微信支付商户平台配置，请前往')}
            <a
              href='https://pay.weixin.qq.com/'
              target='_blank'
              rel='noreferrer'
            >
              {t('微信支付商户平台')}
            </a>
            {t('获取商户号、APPID和API密钥。')}
            <br />
          </Text>
          <Banner
            type='info'
            description={`${t('异步通知地址')}：${props.options.ServerAddress ? removeTrailingSlash(props.options.ServerAddress) : t('网站地址')}/api/wechatpay/notify`}
          />
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Switch
                field='WechatPayEnabled'
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                label={t('启用微信支付')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='WechatPayAppID'
                label={t('微信 APPID')}
                placeholder={t('例如：wxd678efh567hg6787')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='WechatPayMchID'
                label={t('微信支付商户号')}
                placeholder={t('例如：1900000001')}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='WechatPayKey'
                label={t('API 密钥')}
                placeholder={t('请填写微信支付API密钥，敏感信息不显示')}
                type='password'
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='WechatPayMinTopUp'
                label={t('最低充值金额（元）')}
                placeholder={t('例如：1')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='WechatPayNotifyURL'
                label={t('异步通知地址（可选）')}
                placeholder={t('留空则使用默认地址')}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.Input
                field='WechatPayReturnURL'
                label={t('同步返回地址（可选）')}
                placeholder={t('留空则使用默认地址')}
              />
            </Col>
          </Row>
          <Button onClick={submitWechatPaySetting} style={{ marginTop: 16 }}>
            {t('更新微信支付设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
