# LSYL Tunnel 中文文档

这套文档现在按“两层结构”整理：

- `docs/`：交付、部署、运维、普通使用的第一入口
- `docs/internal/`：开发、发布、深度分析和过程资料

如果你是第一次接手项目，先不要从所有文档开始读，按下面顺序就够了。

## 建议先读

1. [系统总流程与运行边界](system-flow-zh.md)
2. [部署与安装指南](deployment-zh.md)
3. [服务端管理员指南](server-admin-zh.md)
4. [客户端用户指南](client-user-zh.md)
5. [配置参考](config-reference-zh.md)

## 按主题补充

- [安全模型](security-model-zh.md)
- [固定项与替换指南](customization-zh.md)
- [Android 移动端一期方案](mobile-android-zh.md)

## 内部与深度资料

开发、发布和深度分析资料已收敛到：

- [内部文档索引](internal/README-zh.md)

其中包括：

- 开发快速开始
- 目录结构与代码边界
- 网络层级深度分析
- Windows 服务边界
- 签名发布与发布验收
- 归档与阶段性设计资料

## 现在的文档边界

`docs/` 根目录只保留“多数人会直接用到”的内容：

- 系统是什么
- 怎么部署
- 怎么运维
- 怎么使用
- 配置项怎么写

`docs/internal/` 则保留“不是第一阅读入口，但后续一定有价值”的内容：

- 开发调试
- 发布签名
- 深入网络边界
- Windows 服务细节
- 阶段性设计与归档
