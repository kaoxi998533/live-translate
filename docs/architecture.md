# 架构说明

Live Translate Platform 按商业控制边界拆分：移动端负责交互，Go API 负责账号、权限、订阅、额度和 OpenAI Realtime client secret。

## 移动端

Expo React Native 应用负责：

- 注册和登录
- 两人同声翻译界面
- 自动监听和按住说话
- 用量与会员状态展示
- 支付入口和本地测试支付状态

移动端保持轻量：它展示服务端返回的权限状态并采集音频，但不决定用户是否有权翻译。

## API

Go API 负责商业规则：

- token 验证
- bcrypt 密码认证
- 订阅状态
- 试用额度
- 每周额度
- 翻译会话生命周期
- 用量流水
- 支付订单和支付通知入口
- OpenAI Realtime client secret 下发

## 数据库

`DATABASE_URL` 存在时，API 启动会连接 Postgres 并执行迁移。核心表包括：

- `users`
- `plans`
- `subscriptions`
- `payment_orders`
- `trial_grants`
- `quota_periods`
- `translation_sessions`
- `usage_events`

没有 `DATABASE_URL` 时，API 会回退到内存 store，方便快速开发和测试。

## 权限流程

1. 用户登录后请求翻译权限。
2. API 读取用户、试用、订阅和本周用量。
3. 只有试用有效或 Premium 有效且仍有剩余额度时，API 才创建翻译会话。
4. 翻译过程中移动端周期性上报用量。
5. API 写入用量流水并重新计算剩余额度。

客户端可以展示剩余额度，但服务端是唯一可信来源。

## 实时翻译流程

1. Android 应用向 Go API 创建平台翻译会话。
2. Go API 检查试用、订阅和每周额度。
3. Android 应用向 Go API 请求 OpenAI Realtime client secret。
4. Go API 使用服务端 `OPENAI_API_KEY` 调用 OpenAI Realtime。
5. Android 应用使用短期 client secret 通过 WebRTC 连接 OpenAI Realtime。
6. 应用保持自动监听，或在按住说话模式下手动提交音频 turn。

## 支付流程

当前本地已支持订单创建和开发环境标记已支付：

1. 用户创建 Premium 订单。
2. 服务端写入 `payment_orders`。
3. 正式环境由微信支付、Stripe 或应用商店回调推进订单状态。
4. 本地开发环境可调用开发接口把订单标记为已支付。
5. 支付成功后创建/更新 `subscriptions`，用户变为 Premium。

微信支付正式上线前必须接入真实签名、验签、证书轮换、退款和对账。
