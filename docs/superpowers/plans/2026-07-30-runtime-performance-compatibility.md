# Runtime Performance Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在协议、配置和日志格式保持兼容的前提下，完成五项运行时性能优化。

**Architecture:** 服务端优化保持在既有认证、日志和内存状态边界内；客户端通过只读 TLS 配置和已验证启动入口减少重复工作；所有虚拟端点共享一个 WinDivert helper 生命周期。每项改动独立测试和提交，最后执行跨端构建验证。

**Tech Stack:** Go 1.25、Go 1.20 Win7 Lite、Windows Walk/WinDivert、JSONL、PowerShell/批处理发布脚本。

## Global Constraints

- `protocol_version` 保持 2，不新增协议字段。
- 服务端和客户端 YAML 结构不变。
- 四类 JSONL 文件、字段、前缀和同步落盘语义不变。
- 不实现认证令牌、连接复用或多路复用。
- Win7 Lite 保持 Go 1.20、386 且不支持虚拟 IP。
- 不修改或提交用户现有的 `src/server/front/index.html` 工作区改动。

---

### Task 1: 标准库 PBKDF2 兼容替换

**Files:**
- Modify: `src/internal/passutil/passutil.go`
- Modify: `src/internal/passutil/passutil_test.go`

**Interfaces:**
- Consumes: Go 1.25 `crypto/pbkdf2.Key(sha256.New, password, salt, iterations, keyLen)`。
- Produces: 现有 `HashPassword`、`VerifyPassword` API 和哈希文本格式保持不变。

- [ ] **Step 1: 写固定向量失败测试**

新增旧实现固定结果测试，并断言 1000 次迭代的派生结果：

```go
func TestPBKDF2SHA256MatchesLegacyVector(t *testing.T) {
    got, err := pbkdf2SHA256([]byte("secret"), []byte("0123456789abcdef"), 1000, 32)
    if err != nil { t.Fatal(err) }
    if hex.EncodeToString(got) != legacyVector { t.Fatalf("derived key changed: %x", got) }
}
```

- [ ] **Step 2: 运行测试并确认因旧签名不返回错误而失败**

Run: `go test ./src/internal/passutil -run 'TestPBKDF2SHA256MatchesLegacyVector' -count=1`

- [ ] **Step 3: 改用标准库**

删除自实现的 HMAC 循环，让 `pbkdf2SHA256` 返回 `([]byte, error)`；`HashPassword` 透传错误，`VerifyPassword` 在错误时返回 `false`。

- [ ] **Step 4: 验证兼容和分配量**

Run: `go test ./src/internal/passutil -count=1`

补充 `testing.AllocsPerRun`，使用较低迭代测试标准库路径分配次数不随迭代数线性增长。

- [ ] **Step 5: 提交**

```text
perf(auth): use standard library PBKDF2
```

### Task 2: 最近事件环形缓冲与日志单次序列化

**Files:**
- Create: `src/server/tunnel/runtime_event_ring.go`
- Create: `src/server/tunnel/runtime_event_ring_test.go`
- Modify: `src/server/tunnel/server.go`
- Modify: `src/server/tunnel/runtime_events.go`
- Modify: `src/server/tunnel/event_logs.go`
- Modify: `src/server/tunnel/event_logs_test.go`
- Modify: `src/server/tunnel/e2e_test.go`

**Interfaces:**
- Produces: `newRuntimeEventRing(capacity int) *runtimeEventRing`、`Append(RuntimeEvent)`、`Snapshot() []RuntimeEvent`。
- Produces: `(*jsonlLog).WriteJSON(data []byte) error`，调用方保证 `data` 是单条无换行 JSON。

- [ ] **Step 1: 写环形缓冲失败测试**

覆盖未满、容量 1 和容量 3 回绕，断言 `Snapshot()` 始终从最旧到最新且返回副本。

- [ ] **Step 2: 运行测试确认类型不存在**

Run: `go test ./src/server/tunnel -run 'TestRuntimeEventRing' -count=1`

- [ ] **Step 3: 实现固定环形缓冲并接入 Server**

`Server` 用 `events *runtimeEventRing` 代替 `recentEvents`、`eventMu`、`maxRecentEvents`；`recordEvent` 调用 `Append`，状态接口调用 `Snapshot`。

- [ ] **Step 4: 写日志字节复用失败测试**

新增 `TestJSONLLogWriteJSONPreservesEncodedBytes`，直接传入带固定键顺序的已编码 JSON 并断言文件行原样保留；再使用捕获的服务日志和日期 JSONL，提取前缀后的 JSON，断言它与文件行字节完全相同，分别覆盖 request、business、entry-traffic、flow-traffic。

- [ ] **Step 5: 运行日志测试确认 `WriteJSON` 尚不存在**

Run: `go test ./src/server/tunnel -run 'TestJSONLLogWriteJSONPreservesEncodedBytes|TestStructuredLogBytesMatchJSONL' -count=1`

- [ ] **Step 6: 实现单次序列化**

每个 `record*Log` 只调用一次 `json.Marshal`，同一 `data` 传给服务日志和 `WriteJSON`。`WriteJSON` 在互斥锁内完成日期轮转和追加换行，拒绝空数据或内部换行。

- [ ] **Step 7: 运行服务端日志与端到端测试**

Run: `go test ./src/server/tunnel -count=1`

- [ ] **Step 8: 提交**

```text
perf(server): bound runtime events and reuse log JSON
```

### Task 3: IP 状态仅定时或容量清理

**Files:**
- Modify: `src/server/tunnel/connection_limiter.go`
- Modify: `src/server/tunnel/connection_limiter_test.go`
- Modify: `src/server/tunnel/server.go`
- Modify: `src/server/tunnel/fail_persistence_test.go`

**Interfaces:**
- `connectionLimiter.snapshot()` 和 `failTracker.stats()` 只读取统计，不清理。
- `failTracker.trackStateLocked()` 仅在 `len(items) >= maxItems` 时调用 `cleanupLocked`。
- 现有 `cleanup()` 继续作为定时清理入口。

- [ ] **Step 1: 写快照无副作用失败测试**

用可控时钟创建过期状态，调用 `snapshot()` 或 `stats()` 后断言状态仍在；调用 `cleanup()` 后断言状态被删除。

- [ ] **Step 2: 写失败追踪新增 IP 不全表清理测试**

在未达到容量时保留一个已过期的其他 IP，新增第二个 IP 后断言旧 IP 仍在；容量触顶后新增第三个 IP，断言过期状态被清除且新增成功。

- [ ] **Step 3: 运行测试确认当前快照/新增路径会提前清理**

Run: `go test ./src/server/tunnel -run 'TestConnectionLimiterSnapshotDoesNotCleanup|TestFailTrackerStatsDoesNotCleanup|TestFailTrackerNewIPOnlyCleansAtCapacity' -count=1`

- [ ] **Step 4: 移除读取和普通新增路径的全表清理**

快照在锁内只复制计数；失败追踪统计只读取；失败追踪仅在容量判断前执行一次全表清理。

- [ ] **Step 5: 运行连接限制、失败追踪和服务端全量测试**

Run: `go test ./src/server/tunnel -count=1`

- [ ] **Step 6: 提交**

```text
perf(server): limit IP cleanup to scheduled and capacity paths
```

### Task 4: Windows 客户端 TLS 缓存与已验证启动

**Files:**
- Modify: `src/client/tunnel/client.go`
- Modify: `src/client/tunnel/status.go`
- Modify: `src/client/tunnel/client_status_test.go`
- Modify: `src/client/gui/api_windows.go`
- Modify: `src/client/gui/process_windows.go`
- Modify: `src/client/lite/lite_windows.go`
- Modify: `src/client/cmd/lsyl-tunnel-client/main.go`

**Interfaces:**
- Produces: `StartVerified(ctx context.Context, cfg Config, serverVersion string, logf transport.LogFunc) (*Client, error)`。
- `Client.tlsConfig` 是启动时创建后不再修改的共享配置。
- `Start` 保持原签名和立即健康检查行为；`StartVerified` 首次健康检查延后 `healthOKInterval`。

- [ ] **Step 1: 写 TLS 配置启动缓存失败测试**

启动客户端后移除 CA 测试文件，再调用内部 TLS 配置访问和转发检查，断言不再读取文件；无效 CA 必须在任何监听创建前让 `Start` 返回错误。

- [ ] **Step 2: 写已验证启动延迟健康检查失败测试**

将健康检查函数替换为计数测试函数，调用 `StartVerified` 后在短于 `healthOKInterval` 的窗口内断言调用次数为 0，再推进周期并断言为 1。

- [ ] **Step 3: 运行客户端核心测试确认失败**

Run: `go test ./src/client/tunnel -run 'TestClientCachesTLSConfig|TestStartVerifiedDelaysInitialHealthCheck' -count=1`

- [ ] **Step 4: 实现统一启动内部函数**

新增私有 `start(ctx, cfg, logf, verified, serverVersion)`；启动最前面构建 TLS 配置。客户端实例方法直接返回缓存配置。健康循环接受是否先等待正常周期。

- [ ] **Step 5: 切换三个 Windows 入口**

GUI 把登录响应版本传入 `StartVerified`；Lite 和 CLI 改用 `CheckLoginResponse` 并调用 `StartVerified`。不改变凭据保存和错误提示。

- [ ] **Step 6: 运行客户端、GUI 和 Win7 相关测试**

Run: `go test ./src/client/... -count=1`

- [ ] **Step 7: 提交**

```text
perf(client): cache TLS config and skip duplicate startup auth
```

### Task 5: 虚拟规则共享单个 helper 会话

**Files:**
- Modify: `src/client/tunnel/client.go`
- Modify: `src/client/tunnel/client_status_test.go`
- Modify: `src/client/tunnel/virtual_redirect_windows_test.go`

**Interfaces:**
- `startVirtualRedirectSessionFn` 每次客户端连接最多调用一次，参数包含全部虚拟规则。
- `Client` 保存一个共享 `virtualRedirectSession` 和成员规则集合。
- 删除单条虚拟规则监听不关闭共享会话；最后一条成员移除或客户端关闭时只关闭一次。

- [ ] **Step 1: 将独立会话测试改为共享会话期望并运行红灯**

两条虚拟规则断言 helper 启动调用 1 次、参数规则数为 2、随机本地端口互不相同。

Run: `go test ./src/client/tunnel -run 'TestStartVirtualForwardsUseSharedRedirectSession' -count=1`

- [ ] **Step 2: 写共享失败和授权取消测试并确认红灯**

共享 `Done()` 异常关闭后，两条虚拟规则都变为 `ForwardListenFailed`、地址清空，普通规则保持监听；授权取消时全部虚拟规则禁用但客户端在有普通规则时继续运行。

- [ ] **Step 3: 实现批量启动和共享生命周期**

准备完所有规则后一次调用 helper；成功后统一绑定并激活。共享 watcher 在异常时复制成员名列表、关闭对应监听并更新状态。`Close` 和最后成员移除使用同一会话关闭路径。

- [ ] **Step 4: 运行虚拟转发和客户端全量测试**

Run: `go test ./src/client/tunnel -count=1`

- [ ] **Step 5: 提交**

```text
perf(client): share one virtual redirect helper session
```

### Task 6: 跨端验证

**Files:**
- Modify only if verification exposes a regression in files owned by Tasks 1-5.

**Interfaces:**
- Produces: 可构建的服务端、标准客户端、Win7 Lite 和安装包。

- [ ] **Step 1: 格式化和静态检查**

Run: `gofmt -w src/internal/passutil/passutil.go src/internal/passutil/passutil_test.go src/server/tunnel/runtime_event_ring.go src/server/tunnel/runtime_event_ring_test.go src/server/tunnel/server.go src/server/tunnel/runtime_events.go src/server/tunnel/event_logs.go src/server/tunnel/event_logs_test.go src/server/tunnel/e2e_test.go src/server/tunnel/connection_limiter.go src/server/tunnel/connection_limiter_test.go src/server/tunnel/fail_persistence_test.go src/client/tunnel/client.go src/client/tunnel/status.go src/client/tunnel/client_status_test.go src/client/tunnel/virtual_redirect_windows_test.go src/client/gui/api_windows.go src/client/gui/process_windows.go src/client/lite/lite_windows.go src/client/cmd/lsyl-tunnel-client/main.go`

Run: `git diff --check`

- [ ] **Step 2: Go 全量测试**

Run: `go test ./src/... -count=1`

- [ ] **Step 3: 发布脚本自检**

Run: `deploy\windows\test\selfcheck.cmd`

- [ ] **Step 4: Win7 Lite 构建**

Run: `deploy\windows\build-win7-lite.cmd`

- [ ] **Step 5: Windows 发布构建**

Run: `release.cmd`

- [ ] **Step 6: 检查交付内容和工作区**

确认 `dist` 保持服务端安装包、普通客户端 kit/安装包、Win7 Lite 和 Android APK 既定结构；确认 `src/server/front/index.html` 未进入任何本次提交。
