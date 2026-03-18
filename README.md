# TrustOps Ecommerce RiskOps Platform

面向字节电商安全场景的后端平台项目骨架。项目主线是风险治理后端工程，AI 能力作为增强模块接入，不替代规则与流程主链路。

## 项目定位

本仓库用于展示「后端开发工程师-电商安全」方向的工程化能力，聚焦：
- 风险事件接入
- 规则判定与证据链
- 运营案件流转与审计留痕
- 中间件与稳定性治理
- AI Copilot 辅助排查

核心表达是：这不是算法实验仓库，而是一个可服务化、可扩展、可运维的后端平台骨架。

## JD 来源说明

本仓库内容源自 `jd-to-offer` 工作区的目标 JD 分析结果，并做了仓库化整理与工程化重排。

- 来源 JD 文件：
  - `/Volumes/passport/简历/滴滴/.worktrees/trustops-expansion/examples/bytedance_2026_ecom_security_backend_jd.md`
- 来源 case 输出：
  - `/Volumes/passport/简历/滴滴/.worktrees/trustops-expansion/cases/bytedance-ecom-security-2026/`

详细内容见 [docs/jd-source.md](docs/jd-source.md)。

## 关键技能点 / 知识点

本仓库重点覆盖以下能力：
- 后端服务工程：分层架构、接口契约、幂等与重试
- 业务抽象能力：风险事件、规则、案件状态机的领域建模
- 中间件能力：MySQL、Redis、RabbitMQ 的协同使用
- 稳定性与安全：审计日志、可追踪链路、失败补偿
- AI 工程化：作为 Copilot 增强能力接入主流程

详细知识树见 [docs/knowledge-points.md](docs/knowledge-points.md)。

## 架构总览

推荐技术栈：
- `Go + Hertz`：主 API 层与风险流程编排
- `Python + FastAPI`：AI Copilot sidecar
- `MySQL + Redis + RabbitMQ`：存储、缓存、异步任务
- `Prometheus / Grafana`：监控与观测

主链路：
1. `risk ingestion` 接收商家/交易/行为事件
2. `rule engine` 执行命中与评分
3. `case workflow` 生成并流转运营案件
4. `operations data APIs` 提供查询与追踪
5. `ai copilot` 输出摘要、解释与排查建议

详细蓝图见 [docs/project-blueprint.md](docs/project-blueprint.md)。

## 模块拆分

- `services/gateway-go`：网关、鉴权、限流、路由、核心业务接口
- `services/worker`：异步任务、重试、回放、补偿
- `services/ai-copilot`：AI 增强服务（摘要、相似案例、排查建议）
- `infra`：部署、观测、配置模板
- `scripts`：本地开发与验证脚本
- `docs`：JD、知识体系、项目蓝图、面试表达

## 里程碑路线图

1. 第 1 周：完成领域模型、基础 API 骨架、案件状态机草案
2. 第 2 周：打通事件接入 -> 规则命中 -> 案件流转
3. 第 3 周：接入 Redis / RabbitMQ，补齐重试、幂等、审计链路
4. 第 4 周：接入 AI Copilot、压测与观测面板

## 目录结构

```text
trustops-ecommerce-riskops-platform/
├── README.md
├── .gitignore
├── docs/
│   ├── jd-source.md
│   ├── knowledge-points.md
│   ├── project-blueprint.md
│   └── interview-assets.md
├── services/
│   ├── gateway-go/
│   ├── ai-copilot/
│   └── worker/
├── infra/
└── scripts/
```

## 当前状态

当前为 phase-3 运行时骨架（可演示后端平台主链路）：
- Go Hertz gateway（env 驱动配置）
  - `GET /healthz`
  - `POST /api/v1/risk/events/ingest`
  - `GET /api/v1/risk/cases/:case_id`
  - `GET /api/v1/risk/ops/metrics`
- 风险案例仓储抽象
  - `MySQL` 持久化
  - `Redis` 案件查询缓存
  - `In-memory` 测试/兜底实现
- phase-3 可靠性能力
  - ingest 幂等：优先使用 `X-Idempotency-Key`，缺省时回退到稳定请求指纹
  - 事务 outbox：案件、幂等记录、outbox、审计日志一次事务写入
  - outbox relay：Gateway 后台轮询待投递事件，失败进入 retry / dead-letter 状态
  - ops metrics：可直接查看 ingest、replay、pending outbox、dead-letter 聚合指标
- Python FastAPI copilot（AI 增强，不替代主流程）
  - `GET /healthz`
  - `POST /copilot/risk/summary`

## 本地运行

### 1) 环境变量

```bash
cp .env.example .env
```

`.env.example` 记录了 demo 所需变量，默认值可直接用于 `docker compose` 本地运行。

### 2) 运行测试（不依赖外部中间件）

```bash
cd services/gateway-go && go test ./...
cd ../ai-copilot && python3 -m pip install --user -r requirements-dev.txt && python3 -m pytest -q
cd ../worker && go test ./...
```

### 3) 单独启动服务

Gateway:

```bash
cd services/gateway-go
go mod download
go run ./cmd/server
```

Copilot:

```bash
cd services/ai-copilot
python3 -m pip install --user -r requirements.txt
uvicorn app.main:app --host 0.0.0.0 --port 8000
```

Worker:

```bash
cd services/worker
go mod download
go run ./cmd/worker
```

### 4) 使用 Docker Compose 启动全栈

```bash
docker compose up --build
```

Compose 会启动 `mysql`、`redis`、`rabbitmq`、`gateway`、`worker`、`copilot`，并自动加载 `infra/mysql/init/001_init.sql` 初始化表结构。
同时会为 MySQL / Redis / RabbitMQ 启用健康检查，Gateway 在 `mysql` 或 `rabbitmq` 不可用时直接启动失败，避免落入“假成功”的内存 / noop 主链路。

## API 快速示例

Gateway health:

```bash
curl http://127.0.0.1:8080/healthz
```

风险事件接入：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/risk/events/ingest \
  -H "X-Idempotency-Key: demo-risk-001" \
  -H "Content-Type: application/json" \
  -d '{"merchant_id":"merchant-1001","event_type":"abnormal_listing_activity","evidence":["sku_spike","ip_anomaly"],"risk_score":0.87}'
```

风险案例查询：

```bash
curl http://127.0.0.1:8080/api/v1/risk/cases/case-risk-001
```

运营指标查询：

```bash
curl http://127.0.0.1:8080/api/v1/risk/ops/metrics
```

Copilot 风险摘要：

```bash
curl -X POST http://127.0.0.1:8000/copilot/risk/summary \
  -H "Content-Type: application/json" \
  -d '{"case_id":"case-risk-001","merchant_id":"merchant-1001","risk_category":"listing_fraud","evidence_items":["sku_spike","ip_anomaly"],"operator_note":"need triage"}'
```

也可以直接运行脚本：

```bash
./scripts/sample_requests.sh
```

## Phase 3 亮点

- 幂等入口：重复请求不会重复建案，适合“网络抖动 + 客户端重试”的真实场景。
- 可靠异步：案件主数据与 outbox 事件同事务提交，避免“库里有数据但队列没消息”。
- 可运维：通过 metrics 可直接演示 pending / dead-letter 状态，方便面试时讲稳定性治理。
- AI 边界清晰：Copilot 只做摘要和建议，不进入主判定链路。

## 依赖说明

- `services/ai-copilot/requirements.txt`：运行时依赖
- `services/ai-copilot/requirements-dev.txt`：测试依赖
