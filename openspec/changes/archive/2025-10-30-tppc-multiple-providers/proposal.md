# OpenSpec Change Proposal: TPPC Multiple Third-Party Providers Support

**Change ID**: 2025-10-30-tppc-multiple-providers
**Created**: 2025-10-30
**Archived**: 2025-10-30
**Status**: ✅ Implemented and Archived
**Type**: Enhancement
**Priority**: High
**Archive Location**: `/openspec/changes/archive/2025-10-30-tppc-multiple-providers/`

## Summary

将原有的单一 packycode 提供商支持升级为 Third-Party Provider Codex (tppc) 多提供商支持系统，实现更灵活、可扩展的第三方代码 AI 服务集成能力。

## Motivation

### Problem Statement

原有的 `packycode` 配置存在以下限制：
1. **单一提供商限制** - 只能配置一个提供商
2. **配置复杂** - 需要配置 11 个字段，包含复杂的嵌套结构
3. **缺乏灵活性** - 无法轻松切换或添加新的提供商
4. **维护困难** - 多个提供商需要独立的配置管理

### Business Value

1. **增强可扩展性** - 支持无限数量的第三方提供商
2. **简化配置** - 减少必需字段，从 11 个减少到 4 个
3. **提高可用性** - 支持主备提供商策略
4. **改善用户体验** - 更直观的数组格式配置

## Design

### Configuration Structure

#### Before (packycode)
```yaml
packycode:
  enabled: false
  base-url: "https://codex-api.packycode.com/v1"
  requires-openai-auth: true
  wire-api: "responses"
  privacy:
    disable-response-storage: true
  defaults:
    model: "gpt-5"
    model-reasoning-effort: "high"
  credentials:
    openai-api-key: "sk-OPENAI-XXXX..."
```

#### After (tppc)
```yaml
tppc:
  providers:
    - name: "packycode"
      enabled: true
      base-url: "https://codex-api.packycode.com/v1"
      api-key: "sk-OPENAI-XXXX..."
    - name: "custom-provider"
      enabled: false
      base-url: "https://custom.example.com/v1"
      api-key: "sk-CUSTOM-XXXX..."
```

### Hard-coded Defaults

所有 tppc providers 自动应用以下默认值：
- `wire-api: "responses"`
- `privacy.disable-response-storage: true`
- `defaults.model: "gpt-5"`
- `defaults.model-reasoning-effort: "high"`

### Architecture Components

1. **Config Layer** (`internal/config/config.go`)
   - `TppcConfig` 结构体
   - `TppcProvider` 结构体
   - `ValidateTppc()` 验证函数
   - `sanitizeTppc()` 标准化函数

2. **Management API** (`internal/api/handlers/management/tppc.go`)
   - GET `/v0/management/tppc` - 获取配置
   - PUT `/v0/management/tppc` - 替换配置
   - PATCH `/v0/management/tppc` - 部分更新

3. **Executor Integration** (`internal/runtime/executor/codex_executor.go`)
   - `getCodexCreds()` - 凭据获取（优先级机制）
   - `getTppcCreds()` - tppc 配置凭据获取
   - Execute/ExecuteStream 方法集成

4. **Model Registration** (`cmd/server/main.go`)
   - `registerTppcModels()` - 模型注册函数
   - `--tppc` CLI 标志
   - 动态模型注册机制

## Implementation

### Phase 1: Core Configuration
- [x] T029: 添加 tppc 配置结构体
- [x] T030: 集成到配置加载流程
- [x] T031: 实现 sanitizeTppc 函数
- [x] T032: 实现 ValidateTppc 函数

### Phase 2: Management Interface
- [x] T033: 创建 tppc 管理处理器
- [x] T034: 注册 API 路由

### Phase 3: Executor Integration
- [x] T035: 修改执行器支持 tppc fallback
- [x] T036: 实现凭据优先级机制

### Phase 4: Model Registration
- [x] T037: 实现模型注册函数
- [x] T038: 添加 CLI 标志支持

### Phase 5: Documentation & Testing
- [x] T039: 更新配置示例
- [x] T040: 创建使用指南
- [x] T041: 更新实际配置
- [x] T042-T045: 创建和运行测试
- [x] T046-T048: 完善文档

## Testing Strategy

### Unit Tests (8/8 PASS)
- 配置加载测试
- 配置验证测试
- 边界条件测试
- 错误处理测试

### Integration Tests (2/2 PASS)
- 多提供商配置测试
- 凭据优先级测试

### End-to-End Tests (4/4 PASS)
- 执行器集成测试
- 凭据获取测试
- 多提供商切换测试

## Backward Compatibility

✅ **100% 向后兼容**
- 现有 `packycode` 配置保持不变
- 原有功能继续正常工作
- tppc 作为独立增强功能

## Migration Path

1. **当前状态**: packycode 和 tppc 并存
2. **建议**: 新项目直接使用 tppc
3. **迁移**: 将 packycode 配置迁移到 tppc 格式
4. **未来**: 可考虑废弃 packycode（需提前规划）

## Performance Impact

### Positive Impacts
- 简化配置减少内存占用
- 高效的凭据获取机制
- 延迟加载优化性能

### No Negative Impact
- 不影响现有性能
- 可选的 tppc 功能不影响原有流程

## Security Considerations

✅ **安全加强**
- API 密钥保护机制不变
- 严格的输入验证
- 管理 API 权限控制一致
- 日志中不暴露敏感信息

## Deployment Considerations

### Configuration Updates
- 更新 config.example.yaml
- 更新 config.yaml 示例
- 更新文档和指南

### CLI Changes
- 新增 `--tppc` 标志
- 保持现有标志不变

### API Changes
- 新增 `/v0/management/tppc` 端点
- 现有 API 保持不变

## Rollback Plan

### Safe Rollback
- tppc 功能可独立禁用
- 保留 packycode 作为 fallback
- 配置可快速回滚

### Emergency Procedures
- 使用现有 packycode 配置
- 禁用 tppc 功能
- 恢复原有配置

## Monitoring & Observability

### Logging
- 记录 tppc provider 状态
- 凭据获取日志
- 模型注册状态

### Metrics
- tppc provider 数量
- 凭据获取成功率
- 模型注册状态

## Success Metrics

### Technical Metrics
- [x] 100% 测试通过率
- [x] 零编译错误
- [x] 向后兼容性验证

### Functional Metrics
- [x] 支持多提供商配置
- [x] 简化配置字段
- [x] 凭据优先级机制

## Open Questions

1. **Provider 负载均衡**: 未来是否需要多提供商间的自动负载均衡？
2. **健康检查**: 是否需要提供商健康状态监控？
3. **Metrics 集成**: 是否需要 per-provider 详细指标？

## Decision

**✅ APPROVED AND IMPLEMENTED**

所有任务已完成，系统已成功实现 tppc 多提供商支持，具备完整的配置、管理、执行器集成、模型注册和测试验证能力。

## References

- [TPPC 完整实现报告](./implementation-report.md)
- [配置示例](../config.example.yaml)
- [使用指南](../../TPPC_README.md)
- [测试报告](./test-report.md)
