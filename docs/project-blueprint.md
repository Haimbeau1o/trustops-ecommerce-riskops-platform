# 项目蓝图（Ecommerce Security & RiskOps）

## 项目摘要

面向电商安全与商家风险运营的后端平台，覆盖风险事件接入、规则策略、运营工单、审计追踪和 AI Copilot 辅助排查。

## 技术栈

- `Go + Hertz`：主 API 与业务编排
- `Python + FastAPI`：AI Copilot sidecar
- `MySQL + Redis + RabbitMQ`：存储、缓存、异步链路
- `Prometheus / Grafana`：可观测性

## 核心模块

1. `Ingress Gateway`
- 鉴权、限流、租户隔离、请求校验

2. `Risk Evaluation Engine`
- 规则命中、风险评分、命中原因输出

3. `Case Workflow`
- 风险案件流转、分配、状态机管理

4. `Operations Data APIs`
- 商家、案件、证据、指标查询

5. `Async Worker`
- 重算、回放、批处理、补偿任务

6. `AI Copilot`
- 风险摘要、相似案件召回、排查建议

## 评测指标建议

- Risk event ingest latency
- Rule hit rate
- False positive review count
- Case handling latency
- Retry success rate
- Copilot suggestion usefulness

## Demo 场景

- 商家异常行为触发风险案件并进入运营处理流
- 运营查看命中原因、证据链和历史动作日志
- 上线专项规则后对比命中率和处置效率
- Copilot 汇总风险轨迹并给出处置建议

## 里程碑

1. 第 1 周：梳理事件模型与案件状态机
2. 第 2 周：打通规则命中与案件流转主链路
3. 第 3 周：补齐缓存、队列、审计日志
4. 第 4 周：接入 Copilot、报表与压测

## 边界原则

- 主判定链路：规则与流程引擎
- 增强链路：AI Copilot

保证项目呈现重点始终是后端平台能力，而非模型实验。
