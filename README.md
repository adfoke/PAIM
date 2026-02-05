# Personal AI Memory (PAIM) - Core Engine
Project Codename: **paim-go**  \\
Architecture: Local-First · Hybrid Storage (Vector + Graph) · Go

## 1. 项目概述 / Overview
PAIM 是为个人 AI 助手设计的本地长期记忆引擎，解决 LLM “聊完即忘” 的痛点，结合语义检索 (Vector) 与逻辑关联 (Graph) 提供上下文召回与事实存储。
- 语言：Go 1.21+
- 存储：SQLite（单文件），CGO 加载 `sqlite-vss` 以支持向量检索
- 记忆层次：感知缓冲区（内存 TTL）、情节记忆（向量索引）、语义记忆（三元组图谱）

## 2. 架构与目录
```
/cmd
  /server           # HTTP API 入口 (/remember, /ask)
/pkg
  /model            # 核心接口与数据结构
  /memory           # 感知缓冲区 (TTL + capacity)
  /engine/distill   # 蒸馏器（默认启发式，可替换 LLM）
  /store
    /sqlite         # SQLite 初始化、schema、日志 CRUD
    /vector         # sqlite-vss 封装
    /graph          # 三元组 CRUD + 1-hop 查询
  /store/store.go   # MemoryEngine: Observe / Recall / Consolidate
```

核心循环：
- **Recall Loop**：User Query → Graph 查找实体 → Vector 查找相关片段 → 混合返回上下文。
- **Consolidation Loop**：缓冲区定时/触发 → 蒸馏为事实 → 写入 Graph & Vector → 清空缓冲。

## 3. 数据库 Schema（自动创建）
- `memory_logs`：原始对话/行为日志。
- `triples`：微型图谱三元组（含唯一约束与索引）。
- `vss_memories` + `vss_payload`（仅在启用 VSS 时）：向量虚拟表与日志关联表。

## 4. 核心接口 (pkg/model)
```go
Observe(ctx, input SensoryInput) error
Recall(ctx, query string, topK int) (*RecalledContext, error)
Consolidate(ctx) error
```
- `SensoryInput{Content, Source, Metadata}`
- `RecalledContext{RelatedLogs, RelatedFacts}`

## 5. 运行与配置
依赖：Go 1.21+，macOS 默认 CGO 已开启。
如需向量检索，准备 `sqlite-vss` 动态库并设置环境变量。

环境变量（带默认值）：
- `PAIM_LISTEN_ADDR` = `:8080`
- `PAIM_DB_PATH` = `paim.db`
- `PAIM_ENABLE_VSS` = `false` (启用向量检索设为 `true`)
- `GO_SQLITE3_EXTENSIONS` = `` (sqlite-vss 动态库路径，当启用 VSS 时必填)
- `PAIM_VECTOR_DIM` = `1536`
- `PAIM_BUFFER_SIZE` = `128`
- `PAIM_BUFFER_TTL` = `30m`
- `PAIM_CONSOLIDATION_EVERY` = `5m`

启动示例：
```bash
cd ~/Documents/GitHub/PAIM
# 如需向量检索：
# export GO_SQLITE3_EXTENSIONS=/path/to/vss0.dylib
# export PAIM_ENABLE_VSS=true

GOPROXY=https://goproxy.cn,direct go run ./cmd/server
```

## 6. HTTP API
### 6.1 /health
- `GET /health` → `200 ok`

### 6.2 /remember
- `POST /remember`
- Body: `{"content": "今天和Alice讨论了向量索引", "source": "chat", "metadata": {...}}`
- 作用：写入日志 + 缓冲区；若启用向量检索则同步写入向量索引。

### 6.3 /ask
- `GET /ask?q=Alice&k=5`
- 返回：`RecalledContext`（graph facts + vector logs）。

## 7. 蒸馏与嵌入
- 默认蒸馏器：`HeuristicDistiller`（若 metadata 含 subject/predicate/object 则生成三元组，否则生成 `source -> notes -> snippet` 低置信度事实）。
- 默认嵌入：`HashEmbedder`（确定性本地哈希向量，占位用；可替换为符合 `EmbeddingClient` 接口的本地/远程嵌入服务）。

## 8. 测试
```bash
cd ~/Documents/GitHub/PAIM
go test ./...
```
（当前无单测文件，命令可用于验证依赖与构建链路）

## 9. 关键提示
- CGO 必须开启，启用向量检索时需正确加载 `sqlite-vss` 扩展。
- 写入 triples 与 vss_memories 时使用事务，防止数据不一致（已在实现中处理）。
- Local First：默认无外部依赖，向量检索与嵌入均可本地化；需要真实嵌入或 LLM 蒸馏时可按接口替换。

## 10. 后续可扩展方向
- 用真实 LLM 替换蒸馏器，产出更高质量三元组。
- 接入本地/远程 Embedding 服务（如 Ollama / OpenAI）替换 HashEmbedder。
- 增强 graph 检索（多 hop、路径评分）与混合排序策略。
- 增加鉴权与多租户隔离。
