# URL Shortener — 设计文档 (v1)

> 日期: 2026-06-15 · 状态: 已确认,开始实现
> 作者背景: 多年 PHP/Laravel 后端,Go 为学习中的新语言。本项目为作品集 + 学习项目。

## 1. 目标

一个用 Go 写的短链接服务 REST API。第一个可用版本 (v1) 范围:

- `POST /api/links` — 提交长 URL,返回短码
- `GET /{code}` — 302 跳转到原始 URL,并累加点击数
- `GET /api/links/{code}/stats` — 查看某条链接的点击统计
- `GET /health` — 健康检查(也是第一步的 "hello world")

## 2. 关键技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| Web 框架 | **标准库 `net/http`** | Go 1.22+ 的 `ServeMux` 已支持方法+路径路由,零依赖,学习价值最高;以后可无痛换 chi(chi 底层就是 net/http)。 |
| 存储 | **MySQL (XAMPP)** | 最贴近生产,顺便学 Go 的 `database/sql`。 |
| 短码生成 | **随机 base62** | 6–7 位 `[a-zA-Z0-9]`,碰撞重试;不可枚举,实现简单。 |
| v1 范围 | **核心 + 点击统计** | 足够像真实服务,又不至于膨胀。 |
| 数据库名 | `url_shortener` | 与项目同名,全小写 snake_case(跨平台安全)。 |
| 字符集 | `utf8mb4` | 支持完整 Unicode(含 emoji),避免老 `utf8` 3 字节坑。 |

## 3. 目录结构

```
url_shortener/
├── go.mod                      # 模块名 + 依赖 (≈ composer.json)
├── go.sum                      # 依赖校验和 (≈ composer.lock)
├── .env.example                # DB 连接模板
├── cmd/
│   └── server/
│       └── main.go             # 入口 / 组装根 (composition root):手动接好所有依赖
├── internal/                   # 私有包,编译器禁止本模块外 import
│   ├── handler/                # HTTP 层  (≈ Controllers)
│   ├── shortener/              # 业务逻辑 (≈ Services/Actions)
│   ├── storage/                # MySQL 数据访问 (≈ Repository)
│   └── model/                  # 纯数据结构体 Link (≈ Model,但只有数据,无 Active Record)
└── migrations/
    └── 001_create_links.sql    # 原生 SQL 表结构
```

**分层哲学:** 按技术角色分层(handler/shortener/storage),贴近 Laravel 心智模型。
另一种常见 Go 风格是「按功能分包」(单个 `links` 包),小服务两种皆可。

## 4. 数据流

```
POST /api/links {"url":"..."}
  handler.CreateLink → shortener.Shorten(url) → storage.Save(link) → MySQL
  ← 201 {"short_url":"http://localhost:8080/abc123X"}

GET /{code}
  handler.Redirect → storage.FindByCode → storage.IncrementClicks → 302 redirect

GET /api/links/{code}/stats
  handler.Stats → storage.FindByCode → 200 {"url":...,"clicks":42,"created_at":...}

GET /health → 200 OK
```

## 5. MySQL 表结构

```sql
CREATE DATABASE IF NOT EXISTS url_shortener
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE TABLE links (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  code        VARCHAR(16) NOT NULL UNIQUE,
  long_url    TEXT NOT NULL,
  clicks      BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

## 6. 配置 (XAMPP 默认)

```
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASSWORD=          # XAMPP 默认 root 无密码(生产环境务必改)
DB_NAME=url_shortener
```

## 7. 实现阶段

1. **基础搭建(本步)**: go module、目录骨架、`GET /health` 跑通。
2. 配置加载 + MySQL 连接 (`database/sql`)。
3. model + storage 层(含 migration)。
4. shortener(随机 base62)+ `POST /api/links`。
5. `GET /{code}` 跳转 + 点击累加。
6. `GET /api/links/{code}/stats`。
7. 测试、README、错误处理打磨。
