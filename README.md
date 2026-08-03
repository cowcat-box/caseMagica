# CaseMagica

当前版本：<strong>v0.3.0</strong> · Beta

一个面向小说创作与 AI 角色扮演游戏的 AI 创作平台。

[English](README.en.md) | 中文

## 简介

CaseMagica 提供：

- 写作工作台：章节管理、资料库（lore）、风格参考、版本管理
- 游戏模式：互动故事、故事导演、记忆系统、图像生成
- AI 集成：Agents、Skills、Subagent Workflows、自动化任务

## 安装与运行

从 [Releases](https://github.com/cowcat-box/caseMagica/releases) 下载对应平台压缩包，解压后运行：

```bash
# macOS / Linux
./casemagica

# Windows
casemagica.exe
```

浏览器会自动打开 `http://localhost:8080`。

## 从源码构建

需要 Go 1.26+、Node.js 20+、pnpm：

```bash
git clone https://github.com/cowcat-box/caseMagica.git
cd caseMagica

# 开发模式（前端热加载）
./scripts/bootstrap.sh

# 或生产构建
./scripts/build.sh
cd output && ./casemagica
```

## License

[Apache-2.0](LICENSE)
