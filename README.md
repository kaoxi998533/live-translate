# Live Translate Platform

商业级手机同声翻译平台，当前优先支持 Android。

## 技术栈

- 移动端：Expo、React Native、TypeScript
- 后端：Go
- 数据：PostgreSQL、Redis
- 支付：微信支付、Stripe，后续可接入 App Store/Google Play 内购和支付宝

## 应用结构

- `apps/mobile`：面向用户的 Android/iOS 手机应用
- `apps/api`：Go API 服务

## 本地开发

```sh
docker compose up -d
npm install
```

```sh
cd apps/api
DATABASE_URL='postgres://live_translate:live_translate@localhost:5432/live_translate?sslmode=disable' \
OPENAI_API_KEY=sk-proj-your-key \
go run ./cmd/api
```

```sh
npm run android:mobile
```

如果你的 Docker 没有 `compose` 子命令，可以使用 `docker-compose up -d`。

Android 是当前优先目标。应用使用 `react-native-webrtc` 直连 OpenAI Realtime，
因此必须运行 Android development build 或原生 Android 构建，不能使用 Expo Go。

Android 模拟器默认访问 `http://10.0.2.2:8080`。真机测试时请通过
`EXPO_PUBLIC_API_BASE_URL` 指向电脑局域网 IP。

## 产品模型

翻译权限由服务端和 Postgres 控制：

- 注册用户可获得短时试用。
- Premium 会员需要有效订阅。
- Premium 会员仍然有每周翻译时长上限。
- 翻译用量会写入用量流水，方便审计和后续计费。
- 没有 `DATABASE_URL` 时会回退到内存 store，方便临时开发；商业测试请使用 Postgres。

## 实时同声翻译

移动端向 Go API 请求 OpenAI Realtime client secret。Go API 在服务端使用
`OPENAI_API_KEY`，主 API Key 不会下发到手机。

产品面向两个人面对面对话。用户选择 A 说的语言和 B 说的语言，模型只在这两种语言
之间判断当前说话语言，并把内容翻译成对方语言。

当前支持两种收音方式：

- `自动监听`：麦克风保持开启，依靠 OpenAI Realtime server VAD 判断停顿并自动翻译。
- `按住说话`：会话保持连接，只在用户按住按钮时收音，松开后提交这一轮语音并请求翻译。

实时参数沿用了原型项目验证过的低延迟配置：server VAD、450 ms 静音阈值、
250 ms 前缀缓冲、`marin` 语音和输入转写。商业版把这些参数放在后端配置里，
不暴露成普通用户选项。

## 支付接入

数据库和后端已经按多支付渠道设计：

- 微信支付：国内市场优先渠道，使用 `/v1/billing/wechat/notify` 接收支付通知。
- Stripe：海外信用卡订阅渠道，使用 `/v1/billing/stripe/webhook` 接收事件。
- App Store/Google Play 内购：用于正式上架 iOS/Android 商店时补齐平台合规。
- 支付宝：作为后续国内补充渠道预留。

微信支付正式上线前，还需要补齐下单、签名、回调验签、证书轮换、退款、对账和订单状态机。

开发环境已经提供订单创建和标记已支付接口，便于在没有微信商户号时验证 Premium 权限和每周额度。
