# 测试脚本

开发期用的脚本，不参与构建。所有脚本都直接打 HTTP API 或驱动浏览器，不依赖任何测试框架。

## 目录

| 目录 | 用途 |
|---|---|
| `cfg/` | 多实例配置，每份一个设备 ID 和接收目录 |
| `e2e/` | 端到端传输测试（Python，直接调 API） |
| `cdp/` | 浏览器侧测试（无头 Edge + DevTools 协议，验证拖拽和界面状态） |
| `diag/` | 历史诊断脚本，排查组播/网段问题时用过 |
| `out_a…out_d/` | 各实例的接收目录，运行时生成，已被 `.gitignore` 忽略 |

## 起多实例

同一个 exe 用环境变量隔离，互不干扰：

```bash
cd bin
LT_CONFIG=D:/gq/test/cfg/a.json ./lantransfer.exe                                      # A 发送端，45800
LT_CONFIG=D:/gq/test/cfg/b.json LT_PORT=45900 LT_AUTO_ACCEPT=1 ./lantransfer.exe       # B 接收端，45900
LT_CONFIG=D:/gq/test/cfg/c.json LT_PORT=46000 LT_AUTO_ACCEPT=1 ./lantransfer.exe       # C 接收端，46000
LT_CONFIG=D:/gq/test/cfg/d.json LT_PORT=46100 LT_AUTO_ACCEPT=1 ./lantransfer.exe       # D 与 C 同名，用于验证消歧
```

| 配置 | 设备名 | 端口 | 接收目录 |
|---|---|---|---|
| `cfg/a.json` | 发送端 | 45800 | `test/out_a` |
| `cfg/b.json` | 接收端 | 45900 | `test/out_b` |
| `cfg/c.json` | 接收端C | 46000 | `test/out_c` |
| `cfg/d.json` | 接收端C（与 c 同名） | 46100 | `test/out_d` |

`LT_AUTO_ACCEPT=1` 跳过接收确认框，无人值守时用。各实例的 `host` 字段必须与本机主机名一致，否则会重新生成设备 ID。

## e2e

| 脚本 | 验证什么 | 用法 |
|---|---|---|
| `drop_e2e.py` | 拖拽链路：协商 → 流式转发 → 落盘字节一致 | `python test/e2e/drop_e2e.py` |
| `drop_slow.py` | 限速上传时进度是否逐段推进 | `python test/e2e/drop_slow.py` |
| `recv_flat.py` | 文件平铺落盘 + 同名冲突（含两个发送端并发） | `python test/e2e/recv_flat.py 45800 46000 45900` |
| `multi_offer.py` | 多人同时发 offer 是否排队而不是被拒 | `python test/e2e/multi_offer.py` |
| `multi_transfer.py` | 三路并发传 8 MB + md5 校验 | `python test/e2e/multi_transfer.py` |
| `progress_probe.py` | 接收端进度是否连续递增、不重复计数 | `python test/e2e/progress_probe.py` |

除 `recv_flat.py` 用参数传端口外，其余脚本里端口是写死的（45800/45900），与 `cfg/` 对应。

测试样本 `test/测试文件_8MB.bin` 不入库，首次运行前自己生成一个：

```bash
dd if=/dev/urandom of=test/测试文件_8MB.bin bs=1M count=8
```

## cdp

用真实 Chromium 验证前端逻辑（拖拽事件、按钮状态），比只测接口可靠。

```bash
# 1. 起一个带调试端口的无头 Edge
"/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" \
  --headless=new --disable-gpu --no-first-run \
  --remote-debugging-port=9222 --user-data-dir=D:\\gq\\test\\edgecdp about:blank

# 2. 在页面里执行表达式，可选截图
node test/cdp/cdp.mjs 9222 http://127.0.0.1:45800/ test/cdp/expr_drop.js D:\\gq\\test\\shot.png 2500
```

| 表达式 | 验什么 |
|---|---|
| `expr_drop.js` | 拖拽基本流程（遮罩显示、传输完成） |
| `expr_drop_target.js` | 拖到设备卡片、多文件 |
| `expr_multi_drop.js` | 多设备时按名字定位卡片 |
| `expr_multi_blank.js` | 多设备拖到空白处应拒绝 |
| `expr_dup_drop.js` | 同名设备按 `IP:端口` 定位 |
| `expr_dragshot.js` | 拖拽高亮状态截图 |
| `expr_title.js` | 传输行的设备提示 |
| `expr_fwstate.js` | 防火墙按钮的文字/颜色/状态 |

**响应式测试必须用 `SHOT_SIZE`**，不能用 `--window-size`：

```bash
SHOT_SIZE=430x760 node test/cdp/cdp.mjs 9222 http://127.0.0.1:45800/ - shot.png 1500
```

Windows 会把真实窗口钳制到最小宽度（约 500px），`--window-size=430` 截出来只是裁剪图，媒体查询根本没触发。`SHOT_SIZE` 走 CDP 的 `Emulation.setDeviceMetricsOverride`，并在截图前打印 `innerWidth x innerHeight` 自证。

`show_queue.py` 会挂住 3 个待确认请求，用于截排队弹窗。

## diag

一次性排查脚本，留作参考：

| 脚本 | 当时的用途 |
|---|---|
| `mc_probe.py` | 原始 socket 收发组播，确认网络本身通不通 |
| `listen.py` | 监听组播 10 秒，看能收到哪些设备 |
| `scan45800.py` | 并发扫多个 /24 网段的 45800 端口 |
| `src_if_test.py` | 强制从指定网卡发组播，验证发送侧 |
| `fake_peer.py` | 用 Python 冒充一个接收端，逐步验证 offer/upload/done |

`fake_peer.py` 里的接收目录仍指向 `D:/gq/test`，移动脚本后无需改动。
