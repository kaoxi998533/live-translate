# CLAUDE.md

本文件给后续 AI 编码助手使用。请优先按这里的产品边界和工程规则工作。

## 产品定位

Live Translate Platform 是商业级手机同声翻译应用，当前优先 Android，界面语言先保持中文。

核心场景不是单向 source/target 翻译，而是两个人面对面对话：

- 用户选择 `A 说的语言`。
- 用户选择 `B 说的语言`。
- 模型只在这两种语言之间判断当前说话语言，并输出给另一方听的翻译。

当前支持两种收音方式：

- 自动监听：麦克风持续开启，依靠 OpenAI Realtime server VAD 判断停顿并自动翻译。
- 按住说话：只在用户按住按钮时收音，松开后提交并翻译。

## 技术栈

- 移动端：Expo、React Native、TypeScript
- 后端：Go
- 数据库：PostgreSQL
- 本地辅助服务：Redis 预留
- 实时翻译：OpenAI Realtime，默认模型 `gpt-realtime`

## 重要工程边界

- OpenAI 主 API Key 只能留在 Go API 服务端，不能写入移动端。
- 移动端只能请求 Go API 获取 OpenAI Realtime ephemeral client secret。
- 翻译权限、试用、Premium、每周额度都必须由服务端强制，不要只在前端判断。
- UI 文案先保持中文。产品名 `Live Translate Platform` 可以保留英文品牌名。
- Android 是当前优先目标；Expo Go 不能运行 `react-native-webrtc`，需要 development build 或原生 Android build。

## 本地运行

```sh
docker compose up -d
npm install
cd apps/api
DATABASE_URL='postgres://live_translate:live_translate@localhost:5432/live_translate?sslmode=disable' \
OPENAI_API_KEY='sk-proj-your-key' \
go run ./cmd/api
```

移动端：

```sh
npm run android:mobile
```

真机测试时需要让 `EXPO_PUBLIC_API_BASE_URL` 指向电脑局域网 IP，例如：

```sh
EXPO_PUBLIC_API_BASE_URL=http://192.168.0.162:8080 npm run android:mobile
```

## 数据和账号

`DATABASE_URL` 存在时，Go API 会连接 Postgres 并执行 `infra/migrations/001_initial.sql`。

账号系统当前包含：

- 邮箱注册/登录
- bcrypt 密码哈希
- 服务端签名 token
- 24 小时试用
- Premium 会员
- 每周额度计算

开发环境仍保留内存 store fallback，方便没有数据库时跑基础测试。

## 支付

当前支付模型按多渠道设计：

- 微信支付：国内优先
- Stripe：海外渠道
- App Store/Google Play 内购：正式上架时需要
- 支付宝：预留

本地开发可以创建订单，并通过开发接口标记已支付来验证 Premium 权限。生产环境不要暴露开发支付接口。

微信支付正式上线必须补齐商户平台侧材料：

- App ID
- 商户号
- API v3 key
- 商户私钥
- 证书序列号
- 可公网访问的通知回调 URL

## 验证命令

```sh
npm run typecheck:mobile
cd apps/api && go test ./...
```

如果改了移动端 UI，优先在连接的 Android 真机上验证。

## 不要做的事

- 不要把用户发来的真实密钥写入 repo。
- 不要把 OpenAI key 打包进移动端。
- 不要把两人对话模型退回成单向 source/target 翻译。
- 不要把额度、订阅、试用只做在前端。
- 不要默认英文 UI。
