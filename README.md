# Hysteria 2 (Fork)

基于 [apernet/hysteria](https://github.com/apernet/hysteria) 的定制分支，针对统一代理平台做了以下改造。

## 与上游的差异

### 1. HTTP 认证器对接统一认证中心

`extras/auth/http.go` — 扩展了 HTTP 认证模块，支持向统一认证中心发起鉴权请求。

配置新增字段：

```yaml
auth:
  type: http
  http:
    url: https://auth-center.example.com/api/v1/auth/verify
    insecure: false
    protocol: "hysteria2"   # 协议标识
    nodeID: "node-01"        # 本节点标识
```

请求/响应格式：

```json
// POST → auth center
{
  "remote_addr": "1.2.3.4:5678",
  "credential": "user:pass",
  "tx": 1048576,
  "protocol": "hysteria2",
  "node_id": "node-01",
  "timestamp": 1716336000
}

// ← 200 OK
{ "ok": true, "id": "user_abc", "msg": "", "ttl": 60 }
```

认证结果带本地缓存（默认 TTL 60s，最大 16384 条），减少对认证中心的压力。

### 2. 优雅重启端点

`extras/trafficlogger/http.go` — 流量统计 HTTP 服务新增 `POST /restart` 端点：

```bash
curl -X POST http://127.0.0.1:8080/restart
# → {"ok":true,"restarted_at":"2026-05-22T10:00:00Z"}
```

`app/cmd/server.go` — 主进程改造为 restart loop，收到重启信号后优雅关闭旧实例再启动新实例，实现零中断配置热加载。

### 3. 仓库规范清理

- 删除上游 `.github/` 目录（ISSUE_TEMPLATE、workflows、FUNDING.yml、dependabot.yml）
- 删除上游 `CHANGELOG.md`
- 本分支不依赖 GitHub Actions，构建与部署通过外部平台管理

## 构建与运行

### 构建

```bash
# 需要 Go 1.21+
cd app
go build -o hysteria .

# 交叉编译（Linux amd64）
GOOS=linux GOARCH=amd64 go build -o hysteria .
```

### 启动

`-c` 支持本地文件路径和远程 URL，启动时自动拉取：

```bash
# 服务端 — 从配置中心远程拉取
./hysteria server -c "https://config-center.example.com/api/v1/node/config?protocol=hysteria2&nodeID=node-01"

# 客户端 — 从配置中心远程拉取
./hysteria client -c "https://config-center.example.com/api/v1/client/config?protocol=hysteria2&nodeID=node-01"

# 也支持本地文件
./hysteria server -c /etc/hysteria/config.yaml
```

> 远程配置：启动时 HTTP GET 拉取，超时 30s，最大 10MB。生产环境中由节点侧 `ConfigPuller` 后台轮询 + `POST /restart` 推送实现热更新。

### 发版

```bash
git tag v2.0.x
git push origin v2.0.x
# → CI 自动构建全平台 → Release → 部署
```

## 分支策略

| 分支 | 用途 |
|------|------|
| `master` | 跟踪上游 `apernet/hysteria:master` |
| `custom-main` | 定制主分支，包含所有改造 |

上游同步：`rebase custom-main onto master`

## 许可证

MIT License（与上游一致）
