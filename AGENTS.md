# AGENTS.md — 协作规则

## 文件访问限制（必须遵守）

**`backend/` 目录已废弃，禁止读写**：不要读取、检索、修改 `backend/` 下的任何文件，也不要在该目录下新建文件。所有后端逻辑的最终实现以 Go 版 autoadmin（`autoadmin/`）和 `dj_agent/` 为准。文档中如需引用历史实现，只允许引用 `docs/` 下的归档文档，不引用 `backend/` 源码。

## 文档同步规则（必须遵守）

**每次代码变更后，必须把受影响功能的最终逻辑更新到对应文档中**，保证文档始终反映当前实现的最终状态，而不是"变更说明"。若该功能尚无文档，则新建。

- 架构/功能文档放 `docs/architecture/`
- 文档写"最终逻辑"（数据流、入口、关键决策、失败语义），不写"本次改了什么"的流水账
- 涉及 Django 与 Go 双实现的功能，文档中需标注两者语义是否对齐及差异点

## API 设计规则（必须遵守）

**删除类 API 只保留批量删除**：列表型资源一律只提供 `POST <资源前缀>/batch-delete/`（body `{"ids":[...]}`），不新增单条删除接口。全部 API/UI/文档约定见 [docs/architecture/CONVENTIONS.md](docs/architecture/CONVENTIONS.md)（含文档组织约定：新增文档前先查 README 索引，主题已存在则并入，过程性内容进 docs/plans 或 docs/archive）。
