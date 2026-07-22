import { lazy, Suspense } from 'react';
import { BrowserRouter, Route, Routes } from 'react-router-dom';

const HomePage = lazy(() => import('@/features/home/pages/HomePage'));
const NotFoundPage = lazy(
  () => import('@/features/not-found/pages/NotFoundPage')
);

function PageLoading() {
  return (
    <div className="grid min-h-48 place-items-center" role="status">
      <span className="text-sm text-muted-foreground">页面加载中…</span>
    </div>
  );
}

export function AppRouter() {
  return (
    <BrowserRouter basename={import.meta.env.BASE_URL}>
      <Suspense fallback={<PageLoading />}>
        <Routes>
          <Route index element={<HomePage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </Suspense>
    </BrowserRouter>
  );
}
