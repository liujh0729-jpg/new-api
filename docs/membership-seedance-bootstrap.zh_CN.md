# 会员等级与 Seedance 生产数据初始化

`bin/init-membership-seedance.py` 用于把 VIP-T1、VIP1-VIP5 和独立 Seedance 管理数据幂等录入最新版 NewAPI。脚本默认只预演，只有增加 `--apply` 才写入；重复执行时，一致项会自动跳过，不会删除配置文件之外的会员等级、模型或渠道。

## 本次冻结的数据口径

会员倍率沿用现生产约定，倍率采用新版会员模块的 PPM 整数格式。会员等级与旧的渠道分组是两个独立维度，脚本不会迁移用户、修改用户分组或自动发放会员。

最终消费会同时乘以“渠道分组倍率”和“会员倍率”。仍在旧 `VIP-T1`、`VIP1` 至 `VIP5` 分组中的用户如果又被发放同名会员，会出现双重折扣；正式发放会员前应先通过会员管理页的迁移预检确认，并按独立迁移流程把旧 VIP 用户和 Token 切回 `default` 分组。

| 等级 | 折扣 | multiplier_ppm | rank |
|---|---:|---:|---:|
| VIP-T1 | 7.3 折 | 730000 | 600 |
| VIP1 | 7.8 折 | 780000 | 500 |
| VIP2 | 8.0 折 | 800000 | 400 |
| VIP3 | 8.5 折 | 850000 | 300 |
| VIP4 | 9.0 折 | 900000 | 200 |
| VIP5 | 9.5 折 | 950000 | 100 |

Seedance 单价的单位均为人民币元/秒。成本与售价取 2026-09-05 读取的生产 AIPDD 管理页：本体成本使用“本体成本矩阵”，展示模型成本使用“服务商成本”，NewAPI 面向用户的售价使用“建议零售价”。页面里的“标准售价”是下游站点结算价，不是 NewAPI 公开售价。初始化 JSON 内部使用整数微元人民币/秒，避免浮点误差。

| 公开模型 | 输入→输出 | AIPDD 成本（无视频/含视频） | NewAPI 售价（无视频/含视频） |
|---|---|---:|---:|
| AP Seedance-2.0 VIP 480p | 480p→480p | ¥0.492 / ¥0.654 | ¥0.600 / ¥0.800 |
| AP Seedance-2.0 VIP 720p | 480p→720p | ¥0.492 / ¥0.654 | ¥0.994 / ¥1.340 |
| AP Seedance-2.0 VIP 1080p | 720p→1080p | ¥1.054 / ¥1.400 | ¥2.478 / ¥3.340 |
| AP Seedance-2.0 VIP 4K | 1080p→4K | ¥2.718 / ¥3.580 | ¥5.054 / ¥7.000 |
| AP Seedance-2.0 标准版 480p | 480p→480p | ¥0.402 / ¥0.530 | ¥0.460 / ¥0.600 |
| AP Seedance-2.0 标准版 720p | 480p→720p | ¥0.402 / ¥0.530 | ¥0.800 / ¥1.050 |
| AP Seedance-2.0 轻量版 480p | 480p→480p | ¥0.262 / ¥0.380 | ¥0.320 / ¥0.440 |
| AP Seedance-2.0 轻量版 720p | 480p→720p | ¥0.262 / ¥0.380 | ¥0.496 / ¥0.700 |
| AP Seedance-2.0 高性价比 1080p | 480p→1080p | ¥0.432 / ¥0.560 | ¥1.600 / ¥2.100 |
| AP Seedance-2.0 高性价比 4K | 480p→4K | ¥0.612 / ¥0.740 | ¥3.200 / ¥4.200 |
| AP Seedance-2.5 标准版 480p | 480p→480p | ¥0.672 / ¥2.018 | ¥0.686 / ¥2.058 |
| AP Seedance-2.5 标准版 720p | 480p→720p | ¥0.702 / ¥2.048 | ¥1.512 / ¥4.536 |
| AP Seedance-2.5 标准版 1080p | 720p→1080p | ¥1.572 / ¥4.596 | ¥3.742 / ¥11.178 |

总成本在最新版 Seedance 三层模型中拆成“Ark 基础生成成本 + 火山 AI MediaKit 处理链路成本”。AIPDD 展示模型的服务商成本与本体成本之差对应当前标准画质链路：≤30 FPS 时 480p/720p 为 ¥0.030/秒、1080p 为 ¥0.060/秒、4K 为 ¥0.240/秒；>30 FPS 按对应规格翻倍。Seedance 2.0 的 480p→480p 仍执行 MediaKit `standard` 画质增强；高性价比 1080p/4K 也复用同一套标准 1080p/4K 处理模型，不再人为拆出“高性价比增强模型”。Seedance 2.5 的原生 480p 不经过处理模型，720p 和 1080p 才进入对应标准链路。配置加载时会校验拆分前后严格守恒，并校验会员计价后售价不低于成本；AP Seedance 480p 沿用现有 `ap_seedance_480p` 规则，不叠加会员折扣，其他规格按最低 VIP-T1 倍率校验。

### 本体模型三个名称的区别

- `火山 Seedance 模型 ID`：实际发送给 Ark 的官方稳定模型名，例如 `doubao-seedance-2-0-fast`。初始化数据不使用 `-260128`、`-260615` 一类日期版本后缀。
- `显示名称`：仅供管理员识别，例如 `seedance-2-0-fast`。成本或超分链路差异不会写进本体模型名称。
- `内部代码`：NewAPI 用于关联成本 revision、处理模型和售卖模型的稳定唯一键。管理页面不显示也不要求填写；手工新增模型时由服务端自动生成，初始化脚本则使用固定代码保证重复执行幂等。它不会发送给火山或展示给客户。

## 首次使用

需要 Python 3.10 或更高版本，以及能够登录目标 NewAPI 的 root 账号。建议通过环境变量传入秘密，不要把密钥写到命令行历史或 JSON 文件。

PowerShell 示例：

```powershell
$env:NEW_API_ADMIN_PASSWORD = '<NewAPI root 密码>'
$env:NEW_API_SEEDANCE_MEDIAKIT_API_KEY = '<火山 AI MediaKit API Key>'
$env:NEW_API_SEEDANCE_ARK_API_KEY = '<火山 Ark API Key>'
$env:NEW_API_SEEDANCE_AIPDD_BILLING_API_KEY = '<AIPDD 后付费 API Key>'
$env:AIPDD_INSTANCE_ID = '<站点实例 UUID>'
$env:AIPDD_BASE_URL = 'https://api.aipdd.example'

python .\bin\init-membership-seedance.py `
  --base-url https://newapi.example.com `
  --channel-id 12
```

预演会登录并读取线上状态，但不会写入。确认计划后执行：

```powershell
python .\bin\init-membership-seedance.py `
  --base-url https://newapi.example.com `
  --channel-id 12 `
  --apply
```

首次写入的新售卖模型默认是停用状态；已有同名模型会保留原启停状态。完成真实凭证和短视频验证后，再显式发布：

```powershell
python .\bin\init-membership-seedance.py `
  --base-url https://newapi.example.com `
  --channel-id 12 `
  --apply `
  --publish
```

如果目标实例还没有类型 59 的 Seedance 渠道，可以加入 `--create-channel`。脚本创建的通用渠道 Key 为约定的占位值 `managed`，真正的 Ark 凭证仍进入独立加密凭证库。

```powershell
python .\bin\init-membership-seedance.py `
  --base-url https://newapi.example.com `
  --create-channel `
  --instance-id '<站点实例 UUID>' `
  --apply
```

机器执行可增加 `--non-interactive`，此时缺少任何必需环境变量都会立即失败。要主动轮换已经存在的 Ark 和 MediaKit 凭证，增加 `--rotate-secrets`；火山费用中心账单同步已经启用时，还必须同时设置：

```text
NEW_API_SEEDANCE_ACCESS_KEY_ID
NEW_API_SEEDANCE_SECRET_ACCESS_KEY
```

## 安全与回滚边界

- API Key 和密码只保存在当前进程内存中，不会进入计划文件、初始化 JSON 或应用前快照。
- 每次 `--apply` 前会在 `data/bootstrap-backups/` 保存去敏线上快照。该目录已被仓库忽略。
- 基础模型和处理模型由 NewAPI 生成不可变新 revision；脚本不会删除旧 revision 或无关数据。
- 初始化失败后不会尝试破坏性的整库回滚。修复报错后重复执行即可继续收敛；需要恢复会员字段或 offering 时，可按应用前快照人工处理。
- `--publish` 要求 AIPDD 财务地址和后付费凭证均已配置，并要求 Ark 凭证在线验证成功。MediaKit 的真实可用性仍应按灰度流程用短视频验证。
- 旧 `vip`、`VIP1` 等用户分组不会自动转成会员授权。如需迁移，先在会员管理页执行迁移预检，再单独确认迁移。

## 可调整配置

默认数据位于 `bin/membership-seedance-bootstrap.production.json`。复制后可用 `--config <文件>` 指定其他快照。每次加载都会检查：代码和公开模型名唯一、VIP rank 顺序、价格为正整数微元、成本矩阵完整、成本拆分守恒、按实际会员计价策略不亏，以及公开名称不暴露内部处理词。

变更售价、成本、基础模型、处理规格或其不可变 revision 行后，脚本会根据完整快照生成新的稳定 `pricing_version`；同一实例上的数据和 revision 不变时，多次运行得到的版本号完全相同。

## 本地验证

```powershell
python -m unittest .\bin\test_init_membership_seedance.py
```
