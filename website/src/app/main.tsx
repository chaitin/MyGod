import { StrictMode } from 'react';
import ReactDOM from 'react-dom/client';
import { App } from '@/app/App';
import '@/styles/globals.css';

const rootElement = document.getElementById('root');

if (!rootElement) {
  throw new Error('找不到应用挂载节点 #root');
}

ReactDOM.createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>
);
