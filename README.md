# proxyipcheck - Cloudflare ProxyIP 检测工具

批量检测 ProxyIP（Cloudflare 反代出口）是否可用，并导出 CSV。

## 原理

对每个 ProxyIP 依次执行：

```
TCP连接(IP:端口) -> HTTP CONNECT 隧道 -> TLS 握手 -> GET https://speed.cloudflare.com/cdn-cgi/trace
```

握手/验证默认使用 **Cloudflare 官方端点**（不会被屏蔽、全球可达）：

- `speed.cloudflare.com/cdn-cgi/trace`：官方 trace 端点，返回 `ip=/colo=/loc=/warp=` 文本，
  一次请求即可同时完成 **可用性验证 + 数据中心(colo) + 地区(loc) + WARP 采集**，HTTPS 延迟取 TLS 握手 RTT。
- `speed.cloudflare.com/__down?bytes=N`：官方下载端点，返回精确 N 字节随机数据，用作**下载测速**。

两者分别由 `-url`（握手）与 `-speedurl`（测速）指定、互不干扰，任何一方被屏蔽都不影响另一方。

> **关于出口 IP**：trace 里的 `ip=` 是节点接入 Cloudflare 的**链路地址**——这类 CF 反代节点
> 常是共享的 IPv6 anycast（如 `2a06:98c0:3600::103`），并非网页实际看到的出口。因此工具会额外发起
> **Web出口核对**（`-webecho`，默认 `ipv4.icanhazip.com`）再取网页真实出口 IPv4（如 `101.32.239.42`）。

- 优先走 **CONNECT 隧道** 模式验证；若隧道被 Cloudflare 边缘拒绝（如 `400 Bad Request`），
  自动降级为 **直连探测**（直接 HTTPS 访问目标 IP，通过 `Server: cloudflare` 或 `CF-RAY` 头判断）。
- 确认为 Cloudflare 反代节点的 IP，会**直连握手 URL（trace 端点）**补全 出口IP/数据中心/地区/WARP/HTTPS延迟；
  直连解析响应 **body 的 trace 文本**（`ip=/colo=/loc=/warp=`，官方 trace 端点对直连同样可用）和
  `cf-meta-*` 响应头；`cf-meta-colo` 缺失时自动从 `CF-RAY` 提取数据中心代号（如 `SIN`、`KIX`）。
- 只有 TCP 通不算有效；两种模式都无 CF 响应的才标为**无效**。
- **失败自动重试 + 不稳定识别**（`-retry`，默认 2）：首次不通的节点会自动重测最多 N 次，
  全部失败才最终判为**无效**；期间若出现"重试后才通过"或"多次失败原因不一致/时通时断"，即使最终
  失败，也会在输出中标注为**不稳定**，并保留**最近一次探测详情**（连通性参考），便于区分
  "稳定不可达"与"偶尔可达"。

## 编译

```powershell
cd C:\Users\hy718\Desktop\ASNIPtest\主程序
go build -o proxyipcheck.exe proxyipcheck.go
```

## 输入文件格式（proxyip.txt）

每行一个地址，支持 `IP:端口` 或 `IP 端口`，支持域名，`#` 开头为注释：

```
# 示例
104.16.0.1:443
172.64.0.1 2053
proxy.example.com:8443
```

> 建议先用优选/扫描工具在当前网络下筛出**可直连**的 CF IP 再放入本文件；
> 若使用记事本保存，UTF-8 的 BOM 已兼容处理，无需担心。

## 使用示例

```powershell
# 快速检测（禁用测速，默认官方端点；PowerShell 建议用等号形式传 0）
proxyipcheck.exe -file proxyip.txt -speedtest=0 -max 20

# 完整检测（含20MB官方下载测速，5路并发）
proxyipcheck.exe -file proxyip.txt -speedtest 5 -delay 800 -max 50

# 自建测速站用法（握手+测速同源；注意PowerShell会把空字符串参数吞掉,请显式传同源URL）
proxyipcheck.exe -file proxyip.txt -url cs.chengdu.cloudns.be -speedurl cs.chengdu.cloudns.be -speedtest 5

# 换端口测试（部分ProxyIP使用2053/8443等端口）
proxyipcheck.exe -file proxyip.txt -cport 2053 -speedtest=0

# 网络差时调大超时
proxyipcheck.exe -file proxyip.txt -timeout 8 -max 10

# 关闭失败重试（加快全量初筛; 注意PowerShell下建议用等号形式传0值）
proxyipcheck.exe -file proxyip.txt -retry=0 -speedtest=0

# 对拿不准的节点多测几轮（结果更接近真实稳定性）
proxyipcheck.exe -file proxyip.txt -retry 3 -retrywait 500 -speedtest=0
```

## 参数说明

| 参数 | 默认 | 说明 |
|------|------|------|
| `-file` | proxyip.txt | ProxyIP 地址文件（`IP:端口` 或 `IP 端口`，支持 `#` 注释） |
| `-outfile` | proxyip.csv | 输出 CSV 文件名；同时自动拆分生成 `{输出文件名}_valid.csv` / `{输出文件名}_invalid.csv`（有效 / 无效含不稳定的，英文名） |
| `-max` | 100 | 并发检测协程数 |
| `-delay` | 0 | HTTPS 延迟阈值(ms)，超过则过滤；0 为禁用 |
| `-speedtest` | 5 | 测速并发协程数；设为 0 禁用测速 |
| `-url` | speed.cloudflare.com/cdn-cgi/trace | 握手/检测 URL（官方 trace 端点，返回 `ip/colo/loc/warp` 文本，全局不被屏蔽）；也可用自建站如 cs.chengdu.cloudns.be（根路径返回100MB下载文件，响应头含 `cf-meta-*` 元数据与 Server-Timing） |
| `-speedurl` | speed.cloudflare.com/__down?bytes=20000000 | 下载测速专用 URL（官方 `__down` 端点）；设为空字符串则与 `-url` 同源 |
| `-bytes` | 20000000 | 测速下载量(字节)，默认 20MB |
| `-cport` | 443 | CONNECT 隧道目标端口 |
| `-timeout` | 5 | TCP 连接超时(秒)，网络不稳定可调大 |
| `-webecho` | ipv4.icanhazip.com | 真实 Web 出口(IPv4)核对回显站；设为空字符串禁用 |
| `-retry` | 2 | 失败自动重试次数：首次不通的节点最多重测 N 次，全部失败才判无效；0 为禁用重试 |
| `-retrywait` | 500 | 重试间隔(毫秒)，避免瞬时抖动下的连续探测误判 |

## 输出

`proxyip.csv`（UTF-8 BOM，Excel 可直接打开）列：

```
IP, 端口, TCP延迟(ms), HTTPS延迟(ms), 出口IP, 出口类型, Web出口IP, 数据中心, 地区, WARP, 速度(MB/s), 状态, 说明
```

结果按"**有效稳定 → 不稳定 → 无效**"分层，层内按 HTTPS 延迟升序、速度降序，有效结果靠前。

> **有效 / 无效自动拆分**：除主 CSV 外，还会按状态额外生成两个 **英文名** 文件（便于脚本/程序识别，避免中文文件名乱码）：
>
> - `{文件名}_valid.csv`：判定为**可用**的节点（含"重试后通过"的不稳定节点）；
> - `{文件名}_invalid.csv`：**不可用**的全部节点（含"时通时断"的不稳定节点、稳定不可达节点）。
>
> 文件名由 `-outfile` 自动派生，默认（`proxyip.csv`）即生成 `proxyip_valid.csv` 和 `proxyip_invalid.csv`；
> 两个文件的表头列与主 CSV 完全一致，"状态"列仍保留 `有效 / 不稳定 / 无效` 便于甄别。

`状态` 列取值：
- **有效**：首次探测即通过；
- **不稳定**：首次失败但靠重试通过，或多轮探测失败原因不一致/期间曾 TCP 建连（时通时断）；
- **无效**：每轮失败原因一致（稳定不可达），或多轮耗尽后仍失败（`-retry 0` 时单轮失败即无效）。

> 无论最终判定如何，"说明"列都会追加**最近一次探测详情**作为连通性参考，如
> `共3次探测均失败[稳定不可达], 最近一次参考: dial tcp 3.0.50.69:443: i/o timeout`。

> `WARP` 列仅在使用 trace 类端点时才有值（默认官方端点即返回；CONNECT 受限、降级为直连探测的
> 节点同样会从 trace body 文本中提取 `warp=`），因此非 trace 端点（如纯测速自建站）时为空白。
>
> - `出口类型`：`出口IP` 列的 IP 族，标注 `(CF链路)`——即 trace/`cf-meta-ip` 看到的接入 Cloudflare 的链路地址。
> - `Web出口IP`：通过 `-webecho` 回显站核对出的**网页实际出口**（如 `101.32.239.42`）；
>   节点拒绝非 Cloudflare 目标（CONNECT 400/EOF）时为空白，并在"说明"里注明"受限"。

## 常见问题

- **默认官方端点（speed.cloudflare.com）打不开**：可换自建/其他测速站，如
  `-url cs.chengdu.cloudns.be -speedurl cs.chengdu.cloudns.be`；握手 URL 与测速 URL 相互独立，可自由组合
  （注意：Windows PowerShell 5 会把空字符串参数吞掉，`-speedurl ""` 请改用 `--%` 传参或显式写同一 URL）。
- **全部 `i/o timeout`**：说明当前网络到这些 IP 的 443 端口根本连不上（常见于中国大陆直连 CF 段被干扰）。
  工具默认对不通节点自动重试 2 次（`-retry`）、全部失败才标"无效"；若标注为**不稳定**，说明探测期间
  出现过可达/不可达波动，请结合"说明"列的**最近一次参考**判断。确定不可达可换一批可直连的 CF IP，
  或用 `-timeout` 调大超时、`-max` 降低并发再试。
- **CONNECT 400 Bad Request**：该 IP 未被 Cloudflare 开放 CONNECT 服务，程序会自动降级为**直连探测**；
  若仍确认是 CF 反代节点则同样标为有效（"直连模式"），并直连测速站补全延迟/出口/地区/速度。
- **测速为 0 但状态有效**：使用了 `-speedtest 0`，或测速连接未建立成功，不影响有效判断。