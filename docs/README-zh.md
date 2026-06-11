# LSYL Tunnel 中文文档

当前版本：`1.1.0`

这套文档按“两层结构”整理：

- `docs/`：交付、部署、运维、普通使用的第一入口。
- `docs/internal/`：开发、发布、深度分析和过程资料。

## 角色入口

| 角色 | 先读文档 | 用途 |
| --- | --- | --- |
| 实施交付 | [部署与安装指南](deployment-zh.md) | 生成安装包、安装服务端、分发客户端。 |
| 服务端管理员 | [服务端管理员指南](server-admin-zh.md) | 管理用户、端口、封禁、日志和服务状态。 |
| 客户端用户 | [客户端用户指南](client-user-zh.md) | 登录、导入配置、连接和排障。 |
| 运维排障 | [系统总流程与运行边界](system-flow-zh.md) | 判断问题位于入口连接、认证、业务控制还是数据流。 |
| 配置维护 | [配置参考](config-reference-zh.md) | 查字段含义、默认值和生产路径。 |
| 继续开发 | [内部文档索引](internal/README-zh.md) | 查源码边界、发布验收、签名和版本升级。 |

## 建议阅读顺序

1. [系统总流程与运行边界](system-flow-zh.md)
2. [部署与安装指南](deployment-zh.md)
3. [服务端管理员指南](server-admin-zh.md)
4. [客户端用户指南](client-user-zh.md)
5. [配置参考](config-reference-zh.md)

## 按主题补充

- [安全模型](security-model-zh.md)
- [固定项与替换指南](customization-zh.md)
- [Android 移动端一期方案](mobile-android-zh.md)
- [版本发布说明](release-notes-zh.md)

## 文档边界

`docs/` 根目录只保留多数人会直接用到的内容：系统是什么、怎么部署、怎么运维、怎么使用、配置项怎么写。

`docs/internal/` 保留维护者资料：开发调试、发布签名、深入网络边界、Windows 服务细节、版本升级、阶段性设计与归档。
