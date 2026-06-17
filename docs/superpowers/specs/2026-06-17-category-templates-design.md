# カテゴリ別テンプレート(規約ベース)設計

- 日付: 2026-06-17
- 対象: `dw`(discussion workspace picker)
- ブランチ: `worktree-feat+category-templates`

## Context（なぜ作るか）

`dw` はClaudeとの議論・調査プロジェクトを `~/Discussion/<category>/<date>-<topic>/README.md`
として素早く作るTUI CLI。現状テンプレートは `~/.config/discussion/template.md` の
**単一ファイル**を起動時に1回読むだけで、カテゴリ(`research`/`incident`/`discussion`/`scratch`)
ごとに出し分けられない。`research` には調査ログ用、`incident` にはタイムライン用、のように
カテゴリごとに最適な雛形を使いたい、というのが動機。

ゴール: **ゼロコンフィグで最初から動き、必要に応じてカテゴリ別テンプレートで拡張できる**。
類似ツール調査では zk(Zettelkasten CLI)の「グループ別テンプレート + `templates/` ディレクトリ」
が最も近い前例で、本設計はその規約ベースの考え方を踏襲する。

## 決定事項（ブレストの結論）

| 論点 | 決定 |
|------|------|
| 粒度 | **カテゴリごと + デフォルト**(topicは無数に増えるので不適) |
| 配置/解決 | **規約ベース・設定ファイルなし**(convention over configuration) |
| 変数 | 現状の `{{title}}` / `{{category}}` / `{{date}}` を**維持**(YAGNI) |
| 拡張UX | **規約のみ・専用コマンドなし**。作り方はREADMEで案内 |

## アーキテクチャ

### テンプレート解決のタイミング変更

中心的な変更は、テンプレート解決を **「起動時に1回」から「カテゴリ確定後」へ移す**こと。
カテゴリは TUI のカテゴリ選択で初めて決まるため、起動時には解決できない。

```
現状: main 起動 → LoadTemplate(単一) → tui.New(tmpl) → Create(tmpl)
新:   main 起動 → tui.New(tmplなし) → onEnter でカテゴリ確定
                                       → ResolveTemplate(category) → Create(tmpl)
```

### 解決順（最初に見つかったものを採用）

基点 `~/.config/discussion/`:

1. `templates/<category>.md` — カテゴリ専用(例 `templates/research.md`)
2. `templates/default.md` — 全カテゴリ共通の既定
3. `template.md` — **既存の単一テンプレート（後方互換）**
4. 組み込み `DefaultTemplate` const — ゼロコンフィグ保証

## コンポーネント別の変更

### `internal/workspace/template.go`
- `ResolveTemplate(category string) string` を新設。
  `~/.config/discussion` と category からパスを組み立て、内部関数へ委譲。
- 解決ロジックは純粋寄りの内部関数 `resolveTemplate(dir, legacyPath, category string) string`
  に分離し、`dir`/`legacyPath` を引数化してテスト容易にする。
- 既存 `LoadTemplate` / `TemplatePath` は段3(後方互換)としてこの中で再利用。
- `RenderTemplate` / `DefaultTemplate` は不変。

### `internal/tui/tui.go`
- `New(...)` から `tmpl` 引数を削除。`Model` は起動時テンプレートを保持しない。
- `onEnter` のカテゴリ確定時、`workspace.Create` 呼び出し直前で
  `tmpl := workspace.ResolveTemplate(r.label)` を解決して渡す。

### `internal/workspace/workspace.go`
- `Create(root, category, topic, now, tmpl string)` の**シグネチャは現状維持**。
  テストで固定テンプレートを渡せる利点を残すため、解決はTUI側で行いCreateは受け取るだけ。

### `main.go`
- 起動時の `LoadTemplate` / `TemplatePath` 配線を削除。`tui.New` を新シグネチャへ。

### `README.md`
- 「`~/.config/discussion/templates/<category>.md` を置くとカテゴリ別になる」
  「`templates/default.md` で全カテゴリ共通の既定」
  「従来の `template.md` も後方互換で有効」を追記。発見可能性は規約のみのためドキュメントで担保。

## エラーハンドリング
- 各段はファイル読み取り失敗(不在含む)で次段へ静かにフォールバック。
  現状 `LoadTemplate` と同じ「沈黙フォールバック」方針を踏襲。最終段の組み込みデフォルトで必ず非空を返す。

## テスト
- `resolveTemplate` の解決順テスト: `t.TempDir()` にテンプレートを段階的に配置し、
  各段(category専用 → default → legacy → 組み込み)が正しく選ばれることを網羅。
  既存の `t.TempDir()` + `t.Setenv("HOME", ...)` + 時間固定パターンに倣う。
- `tui_test.go` の `New(...)` 呼び出しを新シグネチャに追従。
- `Create` 系テストは引数維持のため影響最小(コンパイル追従のみ)。

## 検証（エンドツーエンド）
1. `go test -race ./...` が緑。
2. `make lint` が緑。
3. 手動: `~/.config/discussion/templates/research.md` を置き `dw` で research に作成 →
   その内容が README に反映される。
4. 後方互換: `templates/` 不在 + 既存 `template.md` のみ → 従来どおり `template.md` が使われる。
5. ゼロコンフィグ: 何も置かない → 組み込み `DefaultTemplate` が使われる。

## スコープ外（YAGNI）
- 設定ファイル(TOML/YAML)によるマッピング。
- 変数の追加や text/template 化。
- `dw init` 等のscaffoldコマンド。
- topic単位・作成時選択式のテンプレート。
