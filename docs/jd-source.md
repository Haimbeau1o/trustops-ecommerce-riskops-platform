# JD 来源与拆解依据

## 原始 JD

职位名称：`(26届校招)后端开发工程师-电商安全`

来源文件：
- `/Volumes/passport/简历/滴滴/.worktrees/trustops-expansion/examples/bytedance_2026_ecom_security_backend_jd.md`

原始 JD 核心诉求：
- 负责抖音电商安全业务后端开发
- 支持 B 侧商家产品与风险发现运营产品架构设计与开发
- 参与专项技术调研与新技术引入

## 解析来源

本仓库不是手工拍脑袋规划，基于 `jd-to-offer` 生成链路的 case 输出整理而成：
- `/Volumes/passport/简历/滴滴/.worktrees/trustops-expansion/cases/bytedance-ecom-security-2026/`

其中重点参考：
- `02_knowledge_system.md`
- `04_project_blueprint.md`
- `05_interview_assets.md`

## 为什么落地成 TrustOps 平台

该 JD 的关键词并不指向单点算法 demo，而是指向「风险运营后端平台能力」：
- 后端开发与架构设计
- 风险发现与运营协同
- 业务抽象拆分能力
- 稳定性与中间件工程实践
- AI Coding 作为加分项

因此本仓库采用：
- 平台主链路（规则 + 案件 + 审计 + 数据服务）
- AI 增强模块（Copilot）作为 sidecar

## 范围边界

为了和 JD 强相关，当前骨架明确不优先：
- 大模型训练平台
- 复杂多智能体编排
- 重前端展示工程

优先保证：
- 后端平台架构可讲清
- 关键流程可跑通
- 指标、审计、稳定性可落地
