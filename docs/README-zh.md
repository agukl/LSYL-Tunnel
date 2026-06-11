# LSYL Tunnel 中文文档

当前版本：`1.1.0`

这套文档按“系统功能、实施部署、开发维护、发布管理”整理。阅读入口固定在本页，阶段性过程资料不再作为长期文档保留。

## 角色入口

| 角色 | 先读文档 | 用途 |
| --- | --- | --- |
| 实施交付 | [Windows 部署与安装](deployment/windows-deployment-zh.md) | 生成安装包、安装服务端、分发客户端。 |
| 服务端管理员 | [服务端管理员指南](deployment/server-admin-zh.md) | 管理用户、端口、封禁、日志和服务状态。 |
| 客户端用户 | [客户端用户指南](deployment/client-user-zh.md) | 登录、导入配置、连接和排障。 |
| 运维排障 | [系统总流程与运行边界](system/overview-zh.md) | 判断问题位于入口连接、认证、业务控制还是数据流。 |
| 配置维护 | [配置参考](system/config-reference-zh.md) | 查字段含义、默认值和生产路径。 |
| 继续开发 | [开发快速开始](development/quickstart-zh.md) | 本机开发、源码边界、服务封装和调试。 |
| 版本发布 | [版本号升级手册](release/version-bump-zh.md) | 升级版本号、重建版本资源、跑发布自检。 |

## 系统功能

- [系统总流程与运行边界](system/overview-zh.md)
- [网络连接流程与项目所在层级](system/network-flow-zh.md)
- [安全模型](system/security-zh.md)
- [配置参考](system/config-reference-zh.md)

## 实施部署

- [Windows 部署与安装](deployment/windows-deployment-zh.md)
- [服务端管理员指南](deployment/server-admin-zh.md)
- [客户端用户指南](deployment/client-user-zh.md)
- [Android 移动端说明](deployment/mobile-android-zh.md)
- [固定项与替换指南](deployment/customization-zh.md)

## 开发维护

- [开发快速开始](development/quickstart-zh.md)
- [目录结构与代码边界](development/architecture-zh.md)
- [Windows 服务部署与调试](development/windows-service-zh.md)

## 发布管理

- [版本发布说明](release/release-notes-zh.md)
- [签名发布指南](release/signing-zh.md)
- [正规发布验收清单](release/readiness-checklist-zh.md)
- [版本号升级手册](release/version-bump-zh.md)

## 文档边界

长期文档只保留可反复使用的内容：系统功能、实施部署、开发维护、发布管理。

阶段性计划、旧待办、已实现的过程设计不再保留在主文档树中；对应结论应沉淀到上面四类文档或发布说明里。
