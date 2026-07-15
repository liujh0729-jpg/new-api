<div align="center">

![new-api](/web/default/public/logo.png)

# New API

🍥 **次世代大規模モデルゲートウェイとAI資産管理システム**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.md">English</a> |
  <a href="./README.fr.md">Français</a> |
  <strong>日本語</strong>
</p>

<p align="center">
  <a href="https://raw.githubusercontent.com/Calcium-Ion/new-api/main/LICENSE">
    <img src="https://img.shields.io/github/license/Calcium-Ion/new-api?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/Calcium-Ion/new-api/releases/latest">
    <img src="https://img.shields.io/github/v/release/Calcium-Ion/new-api?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd">
    <img src="https://img.shields.io/badge/docker-Alibaba%20ACR-blue" alt="docker">
  </a><!--
  --><a href="https://goreportcard.com/report/github.com/Calcium-Ion/new-api">
    <img src="https://goreportcard.com/badge/github.com/Calcium-Ion/new-api" alt="GoReportCard">
  </a>
</p>

<p align="center">
  <a href="https://trendshift.io/repositories/20180" target="_blank">
    <img src="https://trendshift.io/api/badge/repositories/20180" alt="QuantumNous%2Fnew-api | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/>
  </a>
  <br>
  <a href="https://hellogithub.com/repository/QuantumNous/new-api" target="_blank">
    <img src="https://api.hellogithub.com/v1/widgets/recommend.svg?rid=539ac4217e69431684ad4a0bab768811&claim_uid=tbFPfKIDHpc4TzR" alt="Featured｜HelloGitHub" style="width: 250px; height: 54px;" width="250" height="54" />
  </a><!--
  --><a href="https://www.producthunt.com/products/new-api/launches/new-api?embed=true&utm_source=badge-featured&utm_medium=badge&utm_campaign=badge-new-api" target="_blank" rel="noopener noreferrer">
    <img src="https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1047693&theme=light&t=1769577875005" alt="New API - All-in-one AI asset management gateway. | Product Hunt" style="width: 250px; height: 54px;" width="250" height="54" />
  </a>
</p>

<p align="center">
  <a href="#-クイックスタート">クイックスタート</a> •
  <a href="#-主な機能">主な機能</a> •
  <a href="#-デプロイ">デプロイ</a> •
  <a href="#-ドキュメント">ドキュメント</a> •
  <a href="#-ヘルプサポート">ヘルプ</a>
</p>

</div>

## 📝 プロジェクト説明

> [!IMPORTANT]
> - 本プロジェクトは個人学習用のみであり、安定性の保証や技術サポートは提供しません。
> - ユーザーは、OpenAIの[利用規約](https://openai.com/policies/terms-of-use)および**法律法規**を遵守する必要があり、違法な目的で使用してはいけません。
> - [《生成式人工智能服务管理暂行办法》](http://www.cac.gov.cn/2023-07/13/c_1690898327029107.htm)の要求に従い、中国地域の公衆に未登録の生成式AI サービスを提供しないでください。

---

## 🤝 信頼できるパートナー

<p align="center">
  <em>順不同</em>
</p>

<p align="center">
  <a href="https://www.cherry-ai.com/" target="_blank">
    <img src="./docs/images/cherry-studio.png" alt="Cherry Studio" height="80" />
  </a><!--
  --><a href="https://github.com/iOfficeAI/AionUi/" target="_blank">
    <img src="./docs/images/aionui.png" alt="Aion UI" height="80" />
  </a><!--
  --><a href="https://bda.pku.edu.cn/" target="_blank">
    <img src="./docs/images/pku.png" alt="北京大学" height="80" />
  </a><!--
  --><a href="https://www.compshare.cn/?ytag=GPU_yy_gh_newapi" target="_blank">
    <img src="./docs/images/ucloud.png" alt="UCloud 優刻得" height="80" />
  </a><!--
  --><a href="https://www.aliyun.com/" target="_blank">
    <img src="./docs/images/aliyun.png" alt="Alibaba Cloud" height="80" />
  </a><!--
  --><a href="https://io.net/" target="_blank">
    <img src="./docs/images/io-net.png" alt="IO.NET" height="80" />
  </a>
</p>

---

## 🙏 特別な感謝

<p align="center">
  <a href="https://www.jetbrains.com/?from=new-api" target="_blank">
    <img src="https://resources.jetbrains.com/storage/products/company/brand/logos/jb_beam.png" alt="JetBrains Logo" width="120" />
  </a>
</p>

<p align="center">
  <strong>感謝 <a href="https://www.jetbrains.com/?from=new-api">JetBrains</a> が本プロジェクトに無料のオープンソース開発ライセンスを提供してくれたことに感謝します</strong>
</p>

---

## 🚀 クイックスタート

> [!TIP]
> **最新の ACR イメージ:** `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest`
> **PostgreSQL イメージ:** `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/postgres:15`
> **Redis イメージ:** `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/redis:latest`
> **ACR プル:** `docker pull crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest`

### デプロイ準備

| コンポーネント | 要件 |
|------|------|
| **ローカルデータベース** | SQLite（Docker は `/data` ディレクトリをマウントする必要があります） |
| **リモートデータベース** | MySQL ≥ 5.7.8 または PostgreSQL ≥ 9.6 |
| **コンテナエンジン** | Docker / Docker Compose |

### 必須パラメータ

| パラメータ | 必須になる場合 | 説明 |
|------|----------------|------|
| `AIPDD_API_KEY` | 内蔵 AIPDD タスクモデルを使う場合 | 上流 AIPDD API Key。[app.aipdd.work](https://app.aipdd.work) で登録して取得してください。設定すると、デフォルト URL `https://api.aipdd.work` の `AIPDD` チャンネルを自動作成または同期し、キーを `X-API-Key` として送信します。 |
| `SQL_DSN` | MySQL/PostgreSQL を使う場合 | データベース接続文字列。デフォルト SQLite では省略できますが、データ永続化のため `/data` をマウントしてください。 |
| `SESSION_SECRET` | 本番または複数インスタンス構成 | 再起動や複数インスタンスでログイン状態を安定させる固定セッションシークレット。 |
| `CRYPTO_SECRET` | Redis または複数インスタンス構成 | 共有キャッシュやインスタンス間データを復号できるようにする固定暗号化シークレット。 |
| `REDIS_CONN_STRING` | 複数インスタンス、共有キャッシュ、タスクポーリング | Redis 接続文字列。単一インスタンスではメモリキャッシュから始められます。 |

AIPDD モデルのリクエストパラメータは [AIPDD ユーザーガイド](./docs/aipdd-user-guide.zh_CN.md) を参照してください。よく使う必須項目は、`aipdd-wan2.2-wanx` が `image` と `prompt`、`aipdd-mimic-motion` が `motion_video` と `appearance_image`、`aipdd-indextts` が `input` と `metadata.audio` です。


### Docker Composeを使用（推奨）

```bash
# プロジェクトをクローン
git clone https://github.com/QuantumNous/new-api.git
cd new-api

# docker-compose.yml 設定を編集
nano docker-compose.yml

# サービスを起動
docker-compose up -d
```

<details>
<summary><strong>Dockerコマンドを使用</strong></summary>

```bash
# ACR にログインして最新の AIPDD イメージをプル
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker pull crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest

# AIPDD イメージを起動し、AIPDD チャンネルを自動設定
# AIPDD_API_KEY は https://app.aipdd.work で登録して取得してください
docker run --name new-api -d --restart always \
  -p 6070:6070 \
  -e TZ=Asia/Shanghai \
  -e AIPDD_API_KEY="your-aipdd-api-key" \
  -v ./data:/data \
  crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest

# SQLiteを使用（デフォルト）
docker run --name new-api -d --restart always \
  -p 6070:6070 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest

# MySQLを使用
docker run --name new-api -d --restart always \
  -p 6070:6070 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/oneapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
```

> **💡 ヒント:** `-v ./data:/data` は現在のディレクトリの `data` フォルダにデータを保存します。絶対パスに変更することもできます：`-v /your/custom/path:/data`

</details>

---

🎉 デプロイが完了したら、`http://localhost:6070` にアクセスして使用を開始してください！

📖 その他のデプロイ方法については[デプロイガイド](https://docs.newapi.pro/ja/docs/installation)を参照してください。

---

## 📚 ドキュメント

<div align="center">

### 📖 [公式ドキュメント](https://docs.newapi.pro/ja/docs) | [![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/QuantumNous/new-api)

</div>

**クイックナビゲーション:**

| カテゴリ | リンク |
|------|------|
| 🚀 デプロイガイド | [インストールドキュメント](https://docs.newapi.pro/ja/docs/installation) |
| ⚙️ 環境設定 | [環境変数](https://docs.newapi.pro/ja/docs/installation/config-maintenance/environment-variables) |
| 📡 APIドキュメント | [APIドキュメント](https://docs.newapi.pro/ja/docs/api) |
| ❓ よくある質問 | [FAQ](https://docs.newapi.pro/ja/docs/support/faq) |
| 💬 コミュニティ交流 | [交流チャネル](https://docs.newapi.pro/ja/docs/support/community-interaction) |

---

## ✨ 主な機能

> 詳細な機能については[機能説明](https://docs.newapi.pro/ja/docs/guide/wiki/basic-concepts/features-introduction)を参照してください。

### 🎨 コア機能

| 機能 | 説明 |
|------|------|
| 🎨 新しいUI | モダンなユーザーインターフェースデザイン |
| 🌍 多言語 | 簡体字中国語、繁体字中国語、英語、フランス語、日本語をサポート |
| 🔄 データ互換性 | オリジナルのOne APIデータベースと完全に互換性あり |
| 📈 データダッシュボード | ビジュアルコンソールと統計分析 |
| 🔒 権限管理 | トークングループ化、モデル制限、ユーザー管理 |

### 💰 支払いと課金

- ✅ オンライン充電（EPay、Stripe）
- ✅ モデルの従量課金
- ✅ キャッシュ課金サポート（OpenAI、Azure、DeepSeek、Claude、Qwenなどすべてのサポートされているモデル）
- ✅ 柔軟な課金ポリシー設定

### 🔐 認証とセキュリティ

- 😈 Discord認証ログイン
- 🤖 LinuxDO認証ログイン
- 📱 Telegram認証ログイン
- 🔑 OIDC統一認証
- 🔍 Key使用量クォータ照会（[neko-api-key-tool](https://github.com/Calcium-Ion/neko-api-key-tool)と併用）



### 🚀 高度な機能

**APIフォーマットサポート:**
- ⚡ [OpenAI Responses](https://docs.newapi.pro/ja/docs/api/ai-model/chat/openai/create-response)
- ⚡ [OpenAI Realtime API](https://docs.newapi.pro/ja/docs/api/ai-model/realtime/create-realtime-session)（Azureを含む）
- ⚡ [Claude Messages](https://docs.newapi.pro/ja/docs/api/ai-model/chat/create-message)
- ⚡ [Google Gemini](https://doc.newapi.pro/ja/api/google-gemini-chat)
- 🔄 [Rerankモデル](https://docs.newapi.pro/ja/docs/api/ai-model/rerank/create-rerank)（Cohere、Jina）

**インテリジェントルーティング:**
- ⚖️ チャネル重み付けランダム
- 🔄 失敗自動リトライ
- 🚦 ユーザーレベルモデルレート制限

**フォーマット変換:**
- 🔄 **OpenAI Compatible ⇄ Claude Messages**
- 🔄 **OpenAI Compatible → Google Gemini**
- 🔄 **Google Gemini → OpenAI Compatible** - テキストのみ、関数呼び出しはまだサポートされていません
- 🚧 **OpenAI Compatible ⇄ OpenAI Responses** - 開発中
- 🔄 **思考からコンテンツへの機能**

**Reasoning Effort サポート:**

<details>
<summary>詳細設定を表示</summary>

**OpenAIシリーズモデル:**
- `o3-mini-high` - 高思考努力
- `o3-mini-medium` - 中思考努力
- `o3-mini-low` - 低思考努力
- `gpt-5-high` - 高思考努力
- `gpt-5-medium` - 中思考努力
- `gpt-5-low` - 低思考努力

**Claude思考モデル:**
- `claude-3-7-sonnet-20250219-thinking` - 思考モードを有効にする

**Google Geminiシリーズモデル:**
- `gemini-2.5-flash-thinking` - 思考モードを有効にする
- `gemini-2.5-flash-nothinking` - 思考モードを無効にする
- `gemini-2.5-pro-thinking` - 思考モードを有効にする
- `gemini-2.5-pro-thinking-128` - 思考モードを有効にし、思考予算を128トークンに設定する
- Gemini モデル名の末尾に `-low` / `-medium` / `-high` を付けることで推論強度を直接指定できます（追加の思考予算サフィックスは不要です）。

</details>

---

## 🤖 モデルサポート

> 詳細については[APIドキュメント - 中継インターフェース](https://docs.newapi.pro/ja/docs/api)

| モデルタイプ | 説明 | ドキュメント |
|---------|------|------|
| 🤖 OpenAI-Compatible | OpenAI互換モデル | [ドキュメント](https://docs.newapi.pro/ja/docs/api/ai-model/chat/openai/createchatcompletion) |
| 🤖 OpenAI Responses | OpenAI Responsesフォーマット | [ドキュメント](https://docs.newapi.pro/ja/docs/api/ai-model/chat/openai/createresponse) |
| 🎨 Midjourney-Proxy | [Midjourney-Proxy(Plus)](https://github.com/novicezk/midjourney-proxy) | [ドキュメント](https://doc.newapi.pro/api/midjourney-proxy-image) |
| 🎵 Suno-API | [Suno API](https://github.com/Suno-API/Suno-API) | [ドキュメント](https://doc.newapi.pro/api/suno-music) |
| 🔄 Rerank | Cohere、Jina | [ドキュメント](https://docs.newapi.pro/ja/docs/api/ai-model/rerank/creatererank) |
| 💬 Claude | Messagesフォーマット | [ドキュメント](https://docs.newapi.pro/ja/docs/api/ai-model/chat/createmessage) |
| 🌐 Gemini | Google Geminiフォーマット | [ドキュメント](https://docs.newapi.pro/ja/docs/api/ai-model/chat/gemini/geminirelayv1beta) |
| 🔧 Dify | ChatFlowモード | - |
| 🎯 カスタム | 完全な呼び出しアドレスの入力をサポート | - |

### 📡 サポートされているインターフェース

<details>
<summary>完全なインターフェースリストを表示</summary>

- [チャットインターフェース (Chat Completions)](https://docs.newapi.pro/ja/docs/api/ai-model/chat/openai/createchatcompletion)
- [レスポンスインターフェース (Responses)](https://docs.newapi.pro/ja/docs/api/ai-model/chat/openai/createresponse)
- [イメージインターフェース (Image)](https://docs.newapi.pro/ja/docs/api/ai-model/images/openai/post-v1-images-generations)
- [オーディオインターフェース (Audio)](https://docs.newapi.pro/ja/docs/api/ai-model/audio/openai/create-transcription)
- [ビデオインターフェース (Video)](https://docs.newapi.pro/ja/docs/api/ai-model/audio/openai/createspeech)
- [エンベッドインターフェース (Embeddings)](https://docs.newapi.pro/ja/docs/api/ai-model/embeddings/createembedding)
- [再ランク付けインターフェース (Rerank)](https://docs.newapi.pro/ja/docs/api/ai-model/rerank/creatererank)
- [リアルタイム対話インターフェース (Realtime)](https://docs.newapi.pro/ja/docs/api/ai-model/realtime/createrealtimesession)
- [Claudeチャット](https://docs.newapi.pro/ja/docs/api/ai-model/chat/createmessage)
- [Google Geminiチャット](https://docs.newapi.pro/ja/docs/api/ai-model/chat/gemini/geminirelayv1beta)

</details>

---

## 🚢 デプロイ

デプロイ要件、必須パラメータ、Docker / Docker Compose コマンドは [クイックスタート](#-クイックスタート) に移動しました。その他のプラットフォーム向け手順は [デプロイガイド](https://docs.newapi.pro/ja/docs/installation) を参照してください。

---

## 🔗 関連プロジェクト

### 上流プロジェクト

| プロジェクト | 説明 |
|------|------|
| [One API](https://github.com/songquanpeng/one-api) | オリジナルプロジェクトベース |
| [Midjourney-Proxy](https://github.com/novicezk/midjourney-proxy) | Midjourneyインターフェースサポート |

### 補助ツール

| プロジェクト | 説明 |
|------|------|
| [neko-api-key-tool](https://github.com/Calcium-Ion/neko-api-key-tool) | キー使用量クォータ照会ツール |
| [new-api-horizon](https://github.com/Calcium-Ion/new-api-horizon) | New API高性能最適化版 |

---

## 💬 ヘルプサポート

### 📖 ドキュメントリソース

| リソース | リンク |
|------|------|
| 📘 よくある質問 | [FAQ](https://docs.newapi.pro/ja/docs/support/faq) |
| 💬 コミュニティ交流 | [交流チャネル](https://docs.newapi.pro/ja/docs/support/community-interaction) |
| 🐛 問題のフィードバック | [問題フィードバック](https://docs.newapi.pro/ja/docs/support/feedback-issues) |
| 📚 完全なドキュメント | [公式ドキュメント](https://docs.newapi.pro/ja/docs) |

### 🤝 貢献ガイド

あらゆる形の貢献を歓迎します！

- 🐛 バグを報告する
- 💡 新しい機能を提案する
- 📝 ドキュメントを改善する
- 🔧 コードを提出する

---

## 📜 ライセンス

このプロジェクトは [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE) の下でライセンスされています。

本プロジェクトは、[One API](https://github.com/songquanpeng/one-api)（MITライセンス）をベースに開発されたオープンソースプロジェクトです。

お客様の組織のポリシーがAGPLv3ライセンスのソフトウェアの使用を許可していない場合、またはAGPLv3のオープンソース義務を回避したい場合は、こちらまでお問い合わせください：[support@quantumnous.com](mailto:support@quantumnous.com)

---

## 🌟 スター履歴

<div align="center">

[![スター履歴チャート](https://api.star-history.com/svg?repos=Calcium-Ion/new-api&type=Date)](https://star-history.com/#Calcium-Ion/new-api&Date)

</div>

---

<div align="center">

### 💖 New APIをご利用いただきありがとうございます

このプロジェクトがあなたのお役に立てたなら、ぜひ ⭐️ スターをください！

**[公式ドキュメント](https://docs.newapi.pro/ja/docs)** • **[問題フィードバック](https://github.com/Calcium-Ion/new-api/issues)** • **[最新リリース](https://github.com/Calcium-Ion/new-api/releases)**

<sub>❤️ で構築された QuantumNous</sub>

</div>
