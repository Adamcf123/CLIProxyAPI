# Tasks: Packycode 提供商支持（代理至 Claude Code）

## Format: `[ID] [P?] [Story] Description`

- 任务必须使用如下格式：`- [ ] T001 [P] [US1] Description with file path`
- Setup/Foundational/Polish 阶段不加 [US?] 标签；用户故事阶段必须加 [US?]

## Path Conventions

- 所有文件路径为仓库根相对路径
- 创建新文件时在描述中使用确切目标路径

## Phase 1: Setup (Shared Infrastructure)

- [X] T001 添加 Packycode 配置结构体到 internal/config/config.go（`type PackycodeConfig` 与 `Config.Packycode` 字段）
- [X] T002 在 internal/config/config.go 的 LoadConfigOptional 中设置 Packycode 默认值与调用 `sanitizePackycode(cfg)`
- [X] T003 在 internal/config/config.go 新增 `sanitizePackycode(cfg *Config)`，校验 `base-url` 非空、`wire-api=responses`、`privacy.disable-response-storage=true`、`requires-openai-auth` 与 `defaults` 合法性
- [X] T004 在 internal/api/server.go 的 UpdateClients 日志统计中加入 Packycode 客户端计数输出（与 codex/openai-compat 统计一致的风格）: `packycodeCount`
- [X] T005 在 internal/api/handlers/management/ 新建 `packycode.go`，实现 GET/PUT/PATCH 处理器，读写 `h.cfg.Packycode` 并持久化
- [X] T006 在 internal/api/server.go 的 registerManagementRoutes 中注册 `/v0/management/packycode` 的 GET/PUT/PATCH 路由

## Phase 2: Foundational (Blocking Prerequisites)

- [X] T007 在 internal/watcher/watcher.go 的 SnapshotCoreAuths 中基于 `cfg.Packycode` 合成一个 coreauth.Auth：`Provider=codex`，`Attributes.api_key=openai-api-key`，`Attributes.base_url=packycode.base-url`
- [X] T008 在 internal/watcher/watcher.go 的 diff/变更摘要中加入 Packycode 相关变化提示（例如 `packycode.enabled/base-url/...`），与现有输出风格一致
- [X] T009 在 README_CN.md 的配置章节追加 `packycode:` 字段示例与说明（参考 specs/001-packycode-packycode-openai/quickstart.md）
- [X] T010 在 MANAGEMENT_API_CN.md/MD 中追加 `/v0/management/packycode` 端点说明（GET/PUT/PATCH），字段与默认值说明；同步英文版 MANAGEMENT_API.md

- [X] T027 新增 CLI 标志以注册 Packycode 模型：
  - 在 `cmd/server/main.go` 增加 `--packycode`（或短别名）布尔标志
  - 行为：当检测到 `cfg.Packycode.enabled=true` 且 `base-url`、`openai-api-key` 合法时，主动将 OpenAI/GPT 模型（如 `gpt-5`、`gpt-5-*`、`gpt-5-codex-*`、`codex-mini-latest`）注册进全局 ModelRegistry（provider 归属 `codex`）
  - 要求：执行时不依赖文件变更事件；若与正常服务一同启动，则在服务启动钩子后立即生效
  - 错误处理：若 `packycode` 配置不完整或校验失败，输出清晰错误并返回非零码

- [X] T028 在服务启动路径补充 Packycode 模型注册的兜底钩子：
  - 在 `sdk/cliproxy/service.go` 的启动/重载回调中，当 `cfg.Packycode.enabled=true` 时，直接调用 ModelRegistry 注册 OpenAI 模型（同 T027 逻辑），确保 `/v1/models` 可见 `gpt-5` 等模型
  - 要求：与 Watcher 的合成 Auth 搭配工作；重复注册需幂等处理（使用稳定 clientID，例如基于 `packycode:codex:<base-url|api-key>` 的短哈希）

## Phase 3: User Story 1 - 启用 Packycode 并成功转接 (Priority: P1) 🎯 MVP

- 独立验收：`config.yaml` 新增 `packycode` 字段并启用后，经 Claude Code 兼容入口发起一次请求，收到有效响应

### Implementation for User Story 1

- [X] T011 [US1] 在 internal/config/config.go 定义 Packycode 配置字段：
  - enabled(bool)、base-url(string, required)、requires-openai-auth(bool, default true)、wire-api(string, fixed "responses")、privacy.disable-response-storage(default true)、defaults.model/defaults.model-reasoning-effort
- [X] T012 [US1] 在 internal/api/handlers/management/packycode.go 实现 `GetPackycode/PutPackycode/PatchPackycode`，调用 `h.persist(c)` 并支持只读 `effective-source`
- [X] T013 [US1] 在 internal/api/server.go 注册路由：`mgmt.GET/PUT/PATCH("/packycode", ...)`
- [X] T014 [US1] 在 internal/watcher/watcher.go 依据 `cfg.Packycode.enabled` 决定是否合成 `coreauth.Auth`，并为其生成稳定 ID（使用现有 idGen）
- [X] T015 [US1] 在 internal/runtime/executor/codex_executor.go 无需改动；通过 watcher 合成的 `Provider=codex` + `base_url` 指向 Packycode 即可直通
- [X] T016 [US1] 在 README_CN.md 增加“使用 Packycode”快速验证步骤（参考 specs/.../quickstart.md）

## Phase 4: User Story 2 - 配置校验与可执行报错 (Priority: P2)

- 独立验收：缺失/无效上游密钥或必填项时，保存被拒并获得可执行修复提示

### Implementation for User Story 2

- [X] T017 [US2] 在 internal/api/handlers/management/packycode.go 的 PUT/PATCH 中做字段校验（base-url 必填、requires-openai-auth=>openai-api-key 必填、wire-api=responses、effort 枚举）并返回 422 with 错误详情
- [X] T018 [US2] 在 internal/config/config.go 的 `sanitizePackycode` 中补充严格校验，返回清晰错误（LoadConfigOptional 时可选→错误提示）
- [X] T019 [US2] 在 docs 与 README_CN.md 提示常见错误与修复（缺密钥/URL/非法 effort）

## Phase 5: User Story 3 - 回退与降级 (Priority: P3)

- 独立验收：Packycode 不可用时，可快速停用并恢复至其他已配置提供商，或向调用方输出明确错误

### Implementation for User Story 3

- [X] T020 [US3] 在 internal/watcher/watcher.go 中，当 `packycode.enabled=false` 时移除对应合成的 Auth（触发 rebindExecutors）
- [X] T021 [US3] 在 internal/runtime/executor/codex_executor.go 的错误分支日志中增强可读性（保留现有输出格式，不含用户内容）
- [X] T022 [US3] 在 README_CN.md 增加“快速停用/恢复”说明与故障定位建议

## Phase N: Polish & Cross-Cutting Concerns

- [ ] T023 [P] 补充 MANAGEMENT_API.md 与 MANAGEMENT_API_CN.md 的示例请求/响应样例（与 contracts/management-packycode.yaml 一致）
- [ ] T024 [P] 在 config.example.yaml 添加 `packycode:` 示例片段（注释形式，与现有风格一致）
- [ ] T025 在 internal/api/handlers/management/config_lists.go 附近增加注释引用新的 packycode 管理文件，便于维护者发现
- [ ] T026 在 .codex/prompts/speckit.* 中如有对 codex/codex-api-key 的文字，增加 Packycode 说明（不改变行为）

## Phase N+1: TPPC Enhancement - Multi-Provider Support

### Archive Information

- **Archive Date**: 2025-10-30
- **Change ID**: 2025-10-30-tppc-multiple-providers
- **Archive Location**: `/openspec/changes/archive/2025-10-30-tppc-multiple-providers/`
- **Status**: ✅ Implemented and Archived
- **Integration**: All TPPC tasks completed and integrated into main codebase

### Implementation for TPPC Enhancement

- [A] T029 [TPPC] 添加 TPPC 配置结构体到 internal/config/config.go（`type TppcConfig` 与 `Config.Tppc` 字段，`type TppcProvider`）
- [A] T030 [TPPC] 在 internal/config/config.go 的 LoadConfigOptional 中设置 TPPC 默认值与调用 `sanitizeTppc(cfg)`
- [A] T031 [TPPC] 在 internal/config/config.go 新增 `sanitizeTppc(cfg *Config)`，校验 providers 数组中的每个 enabled provider 的 name、base-url、api-key 字段
- [A] T032 [TPPC] 在 internal/config/config.go 新增 `ValidateTppc(cfg *Config)` 函数，验证所有 enabled providers 具有必需字段
- [A] T033 [TPPC] 在 internal/api/handlers/management/ 新建 `tppc.go`，实现 GET/PUT/PATCH 处理器，读写 `h.cfg.Tppc` 并持久化
- [A] T034 [TPPC] 在 internal/api/server.go 的 registerManagementRoutes 中注册 `/v0/management/tppc` 的 GET/PUT/PATCH 路由
- [A] T035 [TPPC] 修改 internal/runtime/executor/codex_executor.go 的 `Execute` 与 `ExecuteStream` 方法，支持从 tppc 配置获取凭据作为 fallback
- [A] T036 [TPPC] 在 internal/runtime/executor/codex_executor.go 新增 `getCodexCreds` 与 `getTppcCreds` 方法，实现凭据优先级机制
- [A] T037 [TPPC] 在 cmd/server/main.go 新增 `registerTppcModels` 函数，为所有 enabled tppc providers 注册 OpenAI/GPT 模型
- [A] T038 [TPPC] 在 cmd/server/main.go 添加 `--tppc` CLI 标志，用于主动注册 tppc providers 的模型
- [A] T039 [TPPC] 在 config.example.yaml 添加 `tppc:` 配置示例与详细说明，包含多 providers 配置格式
- [A] T040 [TPPC] 创建 TPPC_README.md 完整使用指南，包含配置示例、迁移说明、最佳实践和常见问题
- [A] T041 [TPPC] 在 config.yaml 更新实际配置，展示 tppc 与 packycode 并存使用方式

### TPPC Testing & Validation

- [A] T042 [TPPC] 创建 tests/internal/config/tppc_config_test.go，包含 8 个配置测试用例覆盖各种场景
- [A] T043 [TPPC] 创建 tests/internal/executor/tppc_end_to_end_test.go，包含 4 个端到端测试验证执行器集成
- [A] T044 [TPPC] 运行完整测试套件验证所有 tppc 功能正常工作
- [A] T045 [TPPC] 验证服务器编译成功，无编译错误或警告

### TPPC Documentation & Examples

- [A] T046 [TPPC] 在 config.example.yaml 添加内置默认值说明（wire-api、privacy、defaults 等硬编码参数）
- [A] T047 [TPPC] 更新项目文档，包含 tppc 多提供商支持的说明和使用示例
- [A] T048 [TPPC] 提供从 packycode 迁移到 tppc 的详细指南和字段映射说明

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1 → Phase 2 → Phase 3 (US1) → Phase 4 (US2) → Phase 5 (US3) → Polish → Phase N+1 (TPPC Enhancement)

### User Story Dependencies

- US1 无依赖（MVP）
- US2 依赖 US1 的配置与接口就绪（校验与错误返回覆盖 PUT/PATCH）
- US3 依赖 US1 的启用路径（用于回退/降级验证）

### TPPC Enhancement Dependencies

- TPPC 阶段独立于原有 packycode 实现，可并行开发
- T029–T032 基础配置（依赖 Phase 1 的配置模式）
- T033–T034 管理接口（依赖 Phase 1 的管理模式）
- T035–T036 执行器集成（依赖 Phase 3 的执行器架构）
- T037–T038 模型注册（依赖 Phase 2 的模型注册机制）
- T039–T048 文档与测试（可与其他阶段并行）

### Within Each User Story

- 合同/管理接口 → 配置→ 路由/合成 Auth → 文档

## Parallel Opportunities

- [P] T005 与 T006 可并行（管理处理器与路由注册分文件修改）
- [P] T001/T002/T003 与 T004 可并行（配置结构/校验与日志统计分别修改）
- [P] 文档类任务（T009/T010/T016/T019/T022/T023/T024/T026）可并行
- [P] TPPC 任务可完全并行开发（T029–T048）
- [P] TPPC 测试与文档任务（T042–T048）可与其他 TPPC 实现任务并行

## Implementation Strategy

### MVP First (User Story 1 Only)

- 完成 T001–T006、T007、T011–T016 后即可验收 US1

### Incremental Delivery

- US2 增强校验与错误消息（T017–T019）
- US3 降级策略与文档（T020–T022）
- TPPC 增强：多提供商支持，完全向后兼容（T029–T048）

### Parallel Team Strategy

- 一人负责管理接口与路由（T005/T006/T012/T013/T017）
- 一人负责配置/合成与运行时（T001–T004/T007/T014/T015/T020/T021）
- 一人负责文档与示例（T009/T010/T016/T019/T022/T023/T024/T026）
- TPPC 增强可独立团队并行开发：
  - 一人负责配置结构与管理接口（T029–T034）
  - 一人负责执行器集成与模型注册（T035–T038）
  - 一人负责测试验证与文档（T039–T048）

### TPPC Enhancement Strategy

#### MVP for TPPC (Minimal Viable Product)
- 完成 T029–T032（基础配置）后即可使用基本 tppc 功能
- 完成 T033–T034（管理接口）后即可通过 API 配置 tppc
- 完成 T035–T036（执行器集成）后即可使用 tppc providers

#### Full TPPC Delivery ✅ COMPLETED AND ARCHIVED
- 完整的多提供商支持：T029–T048 全部完成并归档
- 端到端测试验证：T042–T045 验证所有功能
- 完整文档与迁移指南：T039–T041、T046–T048
- **归档信息**: 变更已移动至 `/openspec/changes/archive/2025-10-30-tppc-multiple-providers/`

#### Backward Compatibility
- TPPC 完全独立于原有 packycode 实现
- 现有 packycode 配置保持不变，继续正常工作
- 提供从 packycode 到 tppc 的平滑迁移路径

## Notes

- 所有新增/修改需遵守"隐私优先与最小化留存"：不持久化用户内容；日志仅记录必要元信息
- 合同变更与实现需保持一致（contracts/management-packycode.yaml）

## TPPC Enhancement Notes

### Key Design Decisions

- **Configuration Simplification**: 从 packycode 的复杂嵌套结构简化为 tppc 的简洁数组格式
- **Hard-coded Defaults**: wire-api、privacy、defaults 等参数通过代码内置，配置更简洁
- **Provider Isolation**: 每个 provider 独立配置、启用/禁用，无相互依赖
- **Backward Compatibility**: 保留原有 packycode 配置不变，tppc 作为增强功能独立工作

### Testing Strategy

- **Unit Tests**: 配置解析和验证逻辑的全面测试
- **Integration Tests**: 执行器和 tppc 集成的端到端测试
- **Manual Testing**: 实际多 providers 配置的验证

### Migration Path

- **Phase 1**: tppc 与 packycode 并存，用户可选择性使用
- **Phase 2**: 鼓励迁移到 tppc 以获得更好多提供商支持
- **Phase 3**: 未来版本可考虑废弃 packycode（需提前规划）

### Performance Considerations

- **Lazy Loading**: tppc providers 仅在需要时加载
- **Efficient Fallback**: 凭据获取使用高效的优先级机制
- **Memory Efficiency**: 配置结构优化，减少内存占用

### Security Considerations

- **API Key Protection**: 遵循现有安全实践，不在日志中暴露敏感信息
- **Input Validation**: 严格的配置验证防止注入攻击
- **Access Control**: 管理 API 权限控制与现有机制一致
