import { Link } from 'react-router-dom';

export default function NotFoundPage() {
  return (
    <main className="grid min-h-screen place-items-center bg-background px-6 text-foreground">
      <section className="max-w-lg text-center">
        <p className="text-sm font-semibold text-primary">404</p>
        <h1 className="mt-2 text-3xl font-bold tracking-tight">页面不存在</h1>
        <p className="mt-3 text-muted-foreground">
          当前地址没有对应页面，请检查链接或返回首页。
        </p>
        <Link
          className="mt-6 inline-flex h-10 items-center justify-center rounded-md bg-foreground px-4 text-sm font-medium text-background transition-opacity hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          to="/"
        >
          返回首页
        </Link>
      </section>
    </main>
  );
}
