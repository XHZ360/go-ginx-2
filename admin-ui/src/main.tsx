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
          borderRadius: 8,
          colorBgLayout: '#f5f7fb',
          colorText: '#111827',
          fontFamily: 'Inter, system-ui, sans-serif',
        },
        components: {
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
