import React from 'react';
import ReactDOM from 'react-dom/client';
import { App as AntApp, ConfigProvider } from 'antd';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from 'react-router-dom';
import { createAdminBrowserRouter } from './router';
import { SessionProvider } from './session';
import 'antd/dist/reset.css';
import './styles.css';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      refetchOnWindowFocus: false,
    },
    mutations: {
      retry: false,
    },
  },
});

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: '#2563eb',
          colorInfo: '#2563eb',
          colorSuccess: '#16a34a',
          colorWarning: '#d97706',
          colorError: '#dc2626',
          colorText: '#111827',
          colorTextSecondary: '#64748b',
          colorBorder: '#e5e7eb',
          colorBgLayout: '#f5f7fb',
          colorBgContainer: '#ffffff',
          borderRadius: 8,
          fontFamily: 'Inter, system-ui, sans-serif',
          controlHeight: 36,
          fontSize: 14,
        },
        components: {
          Layout: {
            headerBg: '#ffffff',
            headerHeight: 64,
            headerPadding: '0 24px',
            siderBg: '#0f172a',
            bodyBg: '#f5f7fb',
            triggerBg: '#1e293b',
            triggerColor: '#e2e8f0',
          },
          Menu: {
            darkItemBg: 'transparent',
            darkSubMenuItemBg: 'transparent',
            darkItemSelectedBg: '#1d4ed8',
            darkItemHoverBg: 'rgba(255, 255, 255, 0.08)',
            itemBorderRadius: 8,
          },
          Button: {
            borderRadius: 7,
            controlHeight: 36,
          },
          Card: {
            borderRadiusLG: 8,
          },
          Modal: {
            borderRadiusLG: 8,
          },
          Table: {
            headerBg: '#f8fafc',
            headerColor: '#475569',
            rowHoverBg: '#f8fafc',
            borderColor: '#eef2f7',
          },
          Pagination: {
            itemActiveBg: '#eff6ff',
          },
        },
      }}
    >
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <SessionProvider>
            <RouterProvider router={createAdminBrowserRouter()} />
          </SessionProvider>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>
  </React.StrictMode>,
);
