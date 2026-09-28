# tdx2db 用法总结

将通达信行情数据导入 DuckDB / ClickHouse 的完整用法，包含本地分时补录命令 `import-min`。

## 一、命令总览

| 命令 | 作用 | 数据来源 | 平台限制 |
| :--- | :--- | :--- | :--- |
| `tdx2db init` | 初始化：导入**日线**（只导日线，不计算指标） | 本地 vipdoc `.day` 文件 | 全平台 |
| `tdx2db import-min` | 补录本地**1 分钟分时**历史（本项目新增） | 本地 `.01` / `.lc1` 文件 | 全平台，纯 Go 解析 |
| `tdx2db cron` | 每日增量：日线 + 股本变迁 + 假期 + 在线板块/代码名称 + 重算前收盘价/复权因子 | 通达信官网 + OpenTDX 在线接口 | 全平台 |
| `tdx2db cron --min` | `cron` 的超集，额外下载并导入**当日 1 分钟分时** | 官网 g4tic 归档（仅近期） | **仅 Linux amd64**（需 datatool） |
| `tdx2db check` | 数据完整性体检：检查全部品种近 N 年日线 / 近 M 个月分时的覆盖情况，生成浏览器红绿网格 HTML 报告 | 本地数据库（只读） | 全平台 |
| `tdx2db version` | 打印版本信息 | — | — |

全局 flag：`--temp <dir>` 指定临时文件父目录（默认 `$TMPDIR`，容量不足时用它兜底）。

数据库 URI：

- DuckDB：`duckdb://[path]`，如 `duckdb://./tdx.db`
- ClickHouse：`clickhouse://[user[:password]@][host][:port][/database][?http_port=8123]`

建议所有命令使用**同一个绝对路径** URI——定时任务的 cwd 与手敲命令时不同，相对路径会悄悄建出新库。

## 二、推荐完整流程

```bash
# ① 初始化：本地日线全量导入（仅日线；指标/因子由后续 cron 计算）
tdx2db init --dburi 'duckdb:///abs/path/tdx.db' --dayfiledir /path/to/vipdoc

# ② 本地历史分时补录（.01 / .lc1，递归扫描；与 init 顺序无关）
tdx2db import-min --dburi 'duckdb:///abs/path/tdx.db' --minfiledir /path/to/vipdoc

# ③ init 后立刻跑一次 cron：补齐 gbbq/节假日/在线数据并计算 basic/factor
tdx2db cron --dburi 'duckdb:///abs/path/tdx.db'

# ④ 之后每日定时（收盘后跑），一条命令即可：
#    --min 是 cron 的超集（含全部日线任务 + 分时增量）
tdx2db cron --dburi 'duckdb:///abs/path/tdx.db' --min
```

要点：

- `init` **只导日线**，不计算任何指标；`cron` 才计算 preclose/换手率/市值/复权因子，所以 ③ 不能省
- `init` 发现库内已有日线数据会直接退出（"无需初始化"），不会重复导入
- 周末/节假日 `cron` 整体跳过（打印"休市，全部任务跳过"），每天定时跑是安全的
- ③④ 可以合并：直接跑一次 `cron --min` 即可（但初次建议先跑 ③ 确认日线链路正常）

## 三、构建

| 构建方式 | 产物能力 |
| :--- | :--- |
| `make build`（需 `unrar`，WSL/Linux 下 `sudo apt install unrar`） | **全功能**：构建时自动下载 TDX datatool 并嵌入，`cron --min` 的分时转档可用 |
| 裸 `go build` | 日线、import-min、cron 均正常；**`cron --min` 的分时转档被跳过**（打印"子命令暂不支持"，下载了 tic 也导不进去） |

平台差异：

| 平台 | 日线 | `cron --min` 分时增量 | `import-min` |
| :--- | :--- | :--- | :--- |
| Linux amd64 | ✅ | ✅（需 `make build`） | ✅ |
| Windows / macOS / Linux arm64 | ✅（native Go 合并） | ❌ 转档跳过 | ✅ |

`tdx/embed/datatool` 是 Linux 二进制、不提交 git（构建时下载），这是平台差异的根源。

## 四、分时数据专项

### import-min（本地历史补录）

```bash
tdx2db import-min --dburi 'duckdb://tdx.db' --minfiledir /path/to/vipdoc [--force]
```

- 递归扫描目录下 `.01` / `.lc1` 文件（文件名需形如 `sh600000.lc1`），传 vipdoc 根目录即可（`.day` 会被忽略）
- 纯 Go 解析 32 字节 minline 记录，不依赖 datatool，任何平台可用
- 表内已有分时数据时**默认拒绝**（导入是纯 INSERT，重复执行会翻倍）；`--force` 解除防呆，不做去重
- 库不存在也可以直接跑（自动建表 + 写 schema 版本）

### cron --min（在线滚动增量）

四条规则：

1. **必须有历史底座**：库内分时表为空时，它不会回补历史，只从**今天**开始、每次攒 1 天（并提示"历史请自行导入"）——历史靠 `import-min`
2. **30 个交易日上限**：缺口 ≥ 30 个交易日（约 42 日历天）直接报错"请手动补齐"；< 30 则缺口有多少补多少（官网归档约保留一个月）
3. **收盘后跑**：当天分时 zip 收盘后才发布，盘中跑分钟链会跳过（日线不受影响）
4. **周末跳过**：与 `cron` 一样，休市日整体不执行

### 断更恢复

| 断更时长 | 做法 |
| :--- | :--- |
| ≤ 30 日历天 | 直接 `cron --min`，无任何特殊操作 |
| 30 ~ 42 日历天 | 仍然直接跑（阈值按交易日算），但尽快，别拖 |
| ≥ 42 日历天（≥30 个交易日） | 官网拿不到 → 用本地 `.lc1` 全历史文件补齐（见下） |

长断更补齐（拿到覆盖缺口的本地 `.lc1` 文件后二选一）：

```bash
# 方案 A：清空重导（最干净，推荐）
duckdb tdx.db "DELETE FROM raw_kline_1min"
tdx2db import-min --dburi 'duckdb://tdx.db' --minfiledir /path/to/vipdoc

# 方案 B：--force 全量重导 + 去重（重叠区间会产生重复行，用 SQL 收拾）
tdx2db import-min --dburi 'duckdb://tdx.db' --minfiledir /path/to/vipdoc --force
duckdb tdx.db "DELETE FROM raw_kline_1min WHERE rowid NOT IN (SELECT min(rowid) FROM raw_kline_1min GROUP BY symbol, datetime)"
```

## 五、数据完整性检查（check）

下载不全 / 忘记更新会导致数据出现一段或多段缺失，`check` 生成单文件 HTML 报告（无外部依赖，浏览器直接打开）：

```bash
tdx2db check --dburi 'duckdb://tdx.db' --out tdx2db-report.html [--years 3] [--minmonths 3]
```

报告内容：

- **红绿网格**：每行一个品种，列 = 股票ID / 名称 / 满足度 + 每个交易日一列
  - 日线：绿=有数据，红=缺失
  - 分时：绿=完整（≥240 分钟），黄=部分（1~239 分钟，悬停可见分钟数），红=无
  - 灰=窗口外（品种上市晚于窗口起点，不判缺失）
- **满足度** = 窗口内有数据的交易日占比（绿 ≥98%、黄 ≥90%、红）
- 点击行 → 该品种的全部**缺失区段**（如 `2026-03-05 ~ 2026-03-12（6 个交易日）`）
- **市场级缺失日 TOP**：某日缺失的品种越多，越像整段下载缺失
- **历史截断警告**：多数品种首个数据日一致且晚于窗口起点时提示"疑似未导入完整历史"
- 交互：日线/分时切换、只显示有缺口的品种（默认开）、搜索、拖拽表头缩放日期区间

注意：

1. `check` 是**只读诊断**，不修改被检查的库；`raw_holidays` 为空时按纯工作日检查并给出警告（法定假日会被误标为缺失）
2. **停牌**期间无行情 bar，个别品种孤立的红可能是停牌而非缺失——市场级缺失日才是下载问题的特征
3. 检查窗口终点 = 今天（含）之前的最近交易日：今天的数据已导入则正常计绿；未导入会整列标红，提示补跑 cron

## 六、表与视图

| 表 / 视图 | 说明 |
| :--- | :--- |
| `raw_kline_daily` | 日线（指数/板块含涨跌家数） |
| `raw_kline_1min` | 1 分钟 K 线 |
| `raw_basic_daily` | 前收盘价、涨跌幅、振幅、换手率、市值（stock + etf） |
| `raw_adjust_factor` | 后复权因子 |
| `raw_gbbq` | 股本变迁（除权除息/份额折算） |
| `raw_holidays` | 假期日历 |
| `raw_symbol_class` | 品种分类（stock/index/etf/...，日线导入后重建） |
| `raw_symbol_name` | 在线代码名称 |
| `raw_tdx_blocks_info` / `raw_tdx_blocks_member` | 在线板块/概念/行业及成分 |
| `_meta` | schema 版本等元信息 |
| `v_stock_{bfq,qfq,hfq}` / `v_etf_{bfq,qfq,hfq}` | 复权视图（不复权/前复权/后复权） |

## 七、常见坑

1. **init ≠ 全部**：`init` 只导日线；不跑 `cron` 就没有 preclose、换手率、复权因子
2. **裸 go build 没有分时增量**：`cron --min` 需要 `make build`（嵌入 datatool），否则静默跳过转档
3. **URI 路径**：定时任务里务必用绝对路径，相对路径会新建库
4. **分时无法重建**：日线可以从官网重新下载，分时不行——导入后请保留原始数据并定期备份
5. **重复导入**：`raw_kline_1min` 无唯一约束，`import-min` 默认防呆拒绝；`--force` 前确认你清楚重叠区间会翻倍
6. **股票代码变更**：历史记录不会随代码变更更新

## 八、事件排查记录

### 2026-09-28 日线残缺事件（当日只有 60 个品种）

**现象**：`cron` 一直显示"日线已是最新 (2026-09-28)"，但 `raw_kline_daily` 里 09-28 只有 60 行（正常一个交易日约 9400+ 行）。

**根因链**：

1. 通达信客户端在**收盘前**（14:14-14:20）做增量同步，把当天不完整的数据写入了本地 vipdoc——只有 60 个品种：59 只无成交 LOF 的占位 bar（OHLC=1.0、成交量=0，TDX 官方对无成交基金即发此 bar）+ 1 只新上市 ETF 的真实 bar
2. 当天库的日线表被从**客户端 vipdoc 目录**重新导入过一次（Windows 侧操作），快照带入了这 60 行盘中半截数据；同时历史深度退化为客户端保留范围（约 2021 年起，sh600000 仅 1250 条 vs 全量包 6395 条）
3. `cron` 的"已是最新"判定只看 `max(date)`（`dailyLatest.Before(LastTradingDay)`，见 workflow/plan.go），看到 09-28 已有数据就永远跳过下载——**残缺的一天永久挡住补全**

**诊断方法**（可复用的排查思路）：

- 逐日行数分布：`SELECT date, count(*) FROM raw_kline_daily WHERE date >= ... GROUP BY date` —— 09-28=60 vs 相邻日 ~9400，异常一目了然
- 残缺行内容比对：占位 bar（1.0/0）与本地 vipdoc 文件逐字节一致 → 数据来源是客户端文件而非官网 zip
- 文件时间戳：vipdoc 中相关 .day 文件的 mtime 落在收盘前 → 客户端盘中同步
- 历史深度比对：库中 sh600000 行数与客户端文件行数一致（1250 条）→ 库被客户端数据重建过
- 无重复行（`GROUP BY date, symbol HAVING count(*)>1` 为空）→ 排除"整目录重复导入"假设

**修复**：删除残缺日三张表的数据 → 重跑 `cron` 重新下载完整日线：

```sql
DELETE FROM raw_kline_daily   WHERE date = DATE '2026-09-28';
DELETE FROM raw_basic_daily   WHERE date = DATE '2026-09-28';
DELETE FROM raw_adjust_factor WHERE date = DATE '2026-09-28';
```

**教训**：

- 不要在**收盘前**从通达信客户端 vipdoc 导入数据（客户端盘中同步只写入部分品种）
- 用全量包（hsjday）初始化的库才是完整历史；从客户端 vipdoc 导入的库历史只有客户端保留的约 5 年
- `cron` 的 max(date) 检查不验证当日完整性——数据健康靠 `check` 报告的逐日行数/市场级缺失日人工监控（完整性检查的代码修复暂缓）
