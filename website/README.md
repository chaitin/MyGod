# MagicChat GitHub Pages

该目录承载 MagicChat 的产品落地页，视觉与内容基于 [designIM](https://github.com/duke-yeah/designIM) 项目，并针对 GitHub Pages 做了资源路径和部署适配。

## 本地开发

```bash
npm ci
npm run dev
```

## 验证

```bash
npm run format:check
npm run check
npm run build
```

生产构建输出到 `website/dist`。GitHub Actions 会在 `main` 分支更新时自动构建并部署。
