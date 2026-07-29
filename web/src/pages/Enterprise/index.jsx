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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Table,
  Button,
  Modal,
  Form,
  Input,
  InputNumber,
  Tag,
  Space,
  Tabs,
  TabPane,
  Popconfirm,
  Select,
  Banner,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const { Option } = Select;

const Enterprise = () => {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState('accounts');

  return (
    <div className='mt-[60px] px-2'>
      <Tabs
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key)}
        type='card'
      >
        <TabPane tab={t('子账号管理')} itemKey='accounts'>
          <SubAccountsTable />
        </TabPane>
        <TabPane tab={t('用量统计')} itemKey='usage'>
          <UsageStatsPanel />
        </TabPane>
      </Tabs>
    </div>
  );
};

/* ==================== Sub-Accounts Table ==================== */

const SubAccountsTable = () => {
  const { t } = useTranslation();
  const [accounts, setAccounts] = useState([]);
  const [loading, setLoading] = useState(false);
  const [total, setTotal] = useState(0);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [keyword, setKeyword] = useState('');
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [quotaModalVisible, setQuotaModalVisible] = useState(false);
  const [selectedAccount, setSelectedAccount] = useState(null);

  const loadAccounts = useCallback(
    async (page = currentPage, size = pageSize, kw = keyword) => {
      setLoading(true);
      try {
        const res = await API.get(
          `/api/enterprise/sub-accounts?p=${page}&page_size=${size}&keyword=${encodeURIComponent(kw)}`,
        );
        if (res.data.success) {
          setAccounts(res.data.data.items || []);
          setTotal(res.data.data.total || 0);
        } else {
          showError(res.data.message);
        }
      } catch (e) {
        showError(e.message);
      } finally {
        setLoading(false);
      }
    },
    [currentPage, pageSize, keyword],
  );

  useEffect(() => {
    loadAccounts();
  }, [currentPage, pageSize]);

  const handleSearch = () => {
    setCurrentPage(1);
    loadAccounts(1, pageSize, keyword);
  };

  const handleDelete = async (id) => {
    try {
      const res = await API.delete(`/api/enterprise/sub-accounts/${id}`);
      if (res.data.success) {
        showSuccess(t('删除成功'));
        loadAccounts();
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(e.message);
    }
  };

  const openEditModal = (record) => {
    setSelectedAccount(record);
    setEditModalVisible(true);
  };

  const openQuotaModal = (record) => {
    setSelectedAccount(record);
    setQuotaModalVisible(true);
  };

  const columns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 60 },
      { title: t('用户名'), dataIndex: 'username', width: 140 },
      { title: t('显示名称'), dataIndex: 'display_name', width: 140 },
      {
        title: t('余额'),
        dataIndex: 'quota',
        width: 120,
        render: (quota) => renderQuota(quota),
      },
      {
        title: t('已使用'),
        dataIndex: 'used_quota',
        width: 120,
        render: (quota) => renderQuota(quota),
      },
      { title: t('请求次数'), dataIndex: 'request_count', width: 100 },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 80,
        render: (status) =>
          status === 1 ? (
            <Tag color='green'>{t('正常')}</Tag>
          ) : (
            <Tag color='red'>{t('禁用')}</Tag>
          ),
      },
      {
        title: t('操作'),
        key: 'actions',
        width: 240,
        render: (_, record) => (
          <Space>
            <Button
              size='small'
              type='tertiary'
              onClick={() => openQuotaModal(record)}
            >
              {t('额度')}
            </Button>
            <Button
              size='small'
              type='tertiary'
              onClick={() => openEditModal(record)}
            >
              {t('编辑')}
            </Button>
            <Popconfirm
              title={t('确定删除该子账号？')}
              content={t('删除后余额不会回收')}
              onConfirm={() => handleDelete(record.id)}
            >
              <Button size='small' type='danger'>
                {t('删除')}
              </Button>
            </Popconfirm>
          </Space>
        ),
      },
    ],
    [t],
  );

  return (
    <div className='mt-4'>
      <div className='flex justify-between items-center mb-4 flex-wrap gap-2'>
        <Space>
          <Input
            placeholder={t('搜索用户名/显示名称')}
            value={keyword}
            onChange={(v) => setKeyword(v)}
            onEnterPress={handleSearch}
            style={{ width: 240 }}
          />
          <Button type='primary' onClick={handleSearch}>
            {t('搜索')}
          </Button>
        </Space>
        <Button type='primary' onClick={() => setCreateModalVisible(true)}>
          {t('创建子账号')}
        </Button>
      </div>

      <Table
        columns={columns}
        dataSource={accounts}
        rowKey='id'
        loading={loading}
        pagination={{
          currentPage,
          pageSize,
          total,
          showSizeChanger: true,
          showQuickJumper: true,
          pageSizeOptions: ['10', '20', '50'],
          onChange: (page, size) => {
            setCurrentPage(page);
            setPageSize(size);
          },
          onShowSizeChange: (current, size) => {
            setCurrentPage(1);
            setPageSize(size);
          },
        }}
      />

      <CreateSubAccountModal
        visible={createModalVisible}
        onCancel={() => setCreateModalVisible(false)}
        onSuccess={() => {
          setCreateModalVisible(false);
          loadAccounts();
        }}
      />

      <EditSubAccountModal
        visible={editModalVisible}
        account={selectedAccount}
        onCancel={() => setEditModalVisible(false)}
        onSuccess={() => {
          setEditModalVisible(false);
          loadAccounts();
        }}
      />

      <AllocateQuotaModal
        visible={quotaModalVisible}
        account={selectedAccount}
        onCancel={() => setQuotaModalVisible(false)}
        onSuccess={() => {
          setQuotaModalVisible(false);
          loadAccounts();
        }}
      />
    </div>
  );
};

/* ==================== Create Sub-Account Modal ==================== */

const CreateSubAccountModal = ({ visible, onCancel, onSuccess }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [form, setForm] = useState({
    username: '',
    password: '',
    display_name: '',
    email: '',
  });

  const handleSubmit = async () => {
    if (!form.username || !form.password) {
      showError(t('用户名和密码为必填'));
      return;
    }
    setLoading(true);
    try {
      const res = await API.post('/api/enterprise/sub-accounts', form);
      if (res.data.success) {
        showSuccess(t('创建成功'));
        setForm({ username: '', password: '', display_name: '', email: '' });
        onSuccess();
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      title={t('创建子账号')}
      visible={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={loading}
    >
      <Form layout='vertical'>
        <Form.Item label={t('用户名')} required>
          <Input
            value={form.username}
            onChange={(v) => setForm({ ...form, username: v })}
          />
        </Form.Item>
        <Form.Item label={t('密码')} required>
          <Input
            type='password'
            value={form.password}
            onChange={(v) => setForm({ ...form, password: v })}
          />
        </Form.Item>
        <Form.Item label={t('显示名称')}>
          <Input
            value={form.display_name}
            onChange={(v) => setForm({ ...form, display_name: v })}
          />
        </Form.Item>
        <Form.Item label={t('邮箱')}>
          <Input
            value={form.email}
            onChange={(v) => setForm({ ...form, email: v })}
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};

/* ==================== Edit Sub-Account Modal ==================== */

const EditSubAccountModal = ({ visible, account, onCancel, onSuccess }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [form, setForm] = useState({
    display_name: '',
    password: '',
    status: 1,
  });

  useEffect(() => {
    if (account) {
      setForm({
        display_name: account.display_name || '',
        password: '',
        status: account.status,
      });
    }
  }, [account]);

  const handleSubmit = async () => {
    if (!account) return;
    setLoading(true);
    try {
      const body = {
        id: account.id,
        display_name: form.display_name,
        status: form.status,
      };
      if (form.password) {
        body.password = form.password;
      }
      const res = await API.put('/api/enterprise/sub-accounts', body);
      if (res.data.success) {
        showSuccess(t('更新成功'));
        onSuccess();
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      title={t('编辑子账号')}
      visible={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={loading}
    >
      <Form layout='vertical'>
        <Form.Item label={t('显示名称')}>
          <Input
            value={form.display_name}
            onChange={(v) => setForm({ ...form, display_name: v })}
          />
        </Form.Item>
        <Form.Item label={t('新密码（留空则不修改）')}>
          <Input
            type='password'
            value={form.password}
            onChange={(v) => setForm({ ...form, password: v })}
          />
        </Form.Item>
        <Form.Item label={t('状态')}>
          <Select
            value={form.status}
            onChange={(v) => setForm({ ...form, status: v })}
          >
            <Option value={1}>{t('正常')}</Option>
            <Option value={2}>{t('禁用')}</Option>
          </Select>
        </Form.Item>
      </Form>
    </Modal>
  );
};

/* ==================== Allocate Quota Modal ==================== */

const AllocateQuotaModal = ({ visible, account, onCancel, onSuccess }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [quota, setQuota] = useState(0);
  const [action, setAction] = useState('allocate');

  useEffect(() => {
    if (visible) {
      setQuota(0);
      setAction('allocate');
    }
  }, [visible]);

  const handleSubmit = async () => {
    if (!account || quota === 0) {
      showError(t('请输入有效的额度'));
      return;
    }
    setLoading(true);
    try {
      const amount = action === 'reclaim' ? -quota : quota;
      const res = await API.post(
        `/api/enterprise/sub-accounts/${account.id}/quota`,
        { quota: amount },
      );
      if (res.data.success) {
        showSuccess(action === 'allocate' ? t('分配成功') : t('回收成功'));
        onSuccess();
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      title={t('额度管理')}
      visible={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={loading}
    >
      {account && (
        <Banner
          type='info'
          description={`${t('当前余额')}: ${renderQuota(account.quota)}`}
          className='mb-4'
        />
      )}
      <Form layout='vertical'>
        <Form.Item label={t('操作类型')}>
          <Select value={action} onChange={(v) => setAction(v)}>
            <Option value='allocate'>{t('分配额度')}</Option>
            <Option value='reclaim'>{t('回收到度')}</Option>
          </Select>
        </Form.Item>
        <Form.Item label={t('额度')}>
          <InputNumber
            value={quota}
            onChange={(v) => setQuota(v)}
            min={0}
            style={{ width: '100%' }}
            placeholder={t('请输入额度')}
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};

/* ==================== Usage Stats Panel ==================== */

const UsageStatsPanel = () => {
  const { t } = useTranslation();
  const [stats, setStats] = useState([]);
  const [loading, setLoading] = useState(false);

  const loadStats = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/enterprise/usage');
      if (res.data.success) {
        setStats(res.data.data || []);
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadStats();
  }, [loadStats]);

  const columns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'user_id', width: 60 },
      { title: t('用户名'), dataIndex: 'username', width: 140 },
      { title: t('显示名称'), dataIndex: 'display_name', width: 140 },
      {
        title: t('余额'),
        dataIndex: 'quota',
        width: 120,
        render: (v) => renderQuota(v),
      },
      {
        title: t('已使用'),
        dataIndex: 'used_quota',
        width: 120,
        render: (v) => renderQuota(v),
      },
      { title: t('请求次数'), dataIndex: 'request_count', width: 100 },
      {
        title: t('总Token'),
        dataIndex: 'total_tokens',
        width: 120,
        render: (v) => (v || 0).toLocaleString(),
      },
      {
        title: t('总消费'),
        dataIndex: 'total_quota_used',
        width: 120,
        render: (v) => renderQuota(v),
      },
    ],
    [t],
  );

  return (
    <div className='mt-4'>
      <div className='mb-4'>
        <Button onClick={loadStats} loading={loading}>
          {t('刷新')}
        </Button>
      </div>
      <Table
        columns={columns}
        dataSource={stats}
        rowKey='user_id'
        loading={loading}
        pagination={false}
      />
    </div>
  );
};

/* ==================== Helpers ==================== */

function renderQuota(quota) {
  if (quota === undefined || quota === null) return '-';
  // Use common.QuotaPerUnit (default 500000) to convert
  const unit = 500000;
  const val = quota / unit;
  return `$${val.toFixed(4)}`;
}

export default Enterprise;
