# dw — discussion workspace picker

![coverage](.github/badges/coverage.svg)
[![release](https://img.shields.io/github/v/release/edge2992/dw)](https://github.com/edge2992/dw/releases/latest)
[![license](https://img.shields.io/github/license/edge2992/dw)](LICENSE)

Claudeとの議論・自律調査プロジェクトを素早く作成・横断できる対話CLI。
`ghq` の「勝手にディレクトリを作る」体験と `fzf` の「あいまいジャンプ」を、
非gitの調査ディレクトリ向けに1つにまとめたもの。

プロジェクトは次のレイアウトで管理する:

```
$DISCUSSION_ROOT/<category>/<YYYY-MM-DD>-<topic-slug>/
  README.md   # frontmatter付きインデックス
```

`$DISCUSSION_ROOT` 既定は `~/Discussion`。

## 使い方

`dw` を起動すると既存プロジェクトの**あいまいリスト**が出る(日付の新しい順、
日付なしディレクトリは末尾)。**直前に選んだプロジェクトは最上段にピン留め**され
(`←前回` マーカー付き)、そのまま `enter` で前回の続きに戻れる。

- 打って絞り込み → `enter` で既存プロジェクトへ **cd**
- 選択中の行の下に `status` / `tags` / `created`(README frontmatter)を表示
- マッチが無ければ `+ 作成: <date>-<slug>` が出る → `enter` で **category選択** → 作成して cd
  - ディレクトリ名はスラッグ化、README の `title` は打ったトピックそのまま
- category選択でも未知の名前を打てば新カテゴリを作れる
- category選択で `esc` を押すと browse に**戻る**(トピックの打ち直しが効く)
- `↑/↓`(`ctrl+p/n`)で移動、browse で `esc`/`ctrl+c` で中止

`dw -` は TUI を開かず、**直前のプロジェクトへ即ジャンプ**する(`cd -` 感覚)。

`dw` 単体ではシェルの作業ディレクトリは変えられないため(子プロセスのため)、
選んだパスは **stdout** に出力する。`cd` はシェル側のラッパー関数が行う。

```zsh
function dw() {
  local dir
  dir=$(command dw "$@") && [ -n "$dir" ] && cd "$dir"
}
```

cd した先で Claude をそのまま起動したいなら、薄いラッパーをもう1つ足す:

```zsh
function dwc() {
  local dir
  dir=$(command dw "$@") && [ -n "$dir" ] && cd "$dir" && claude
}
```

直前に選んだプロジェクトは `os.UserCacheDir()`(macOS は `~/Library/Caches/dw/last`、
Linux は `~/.cache/dw/last`)に記録され、ピン留めと `dw -` の両方で使われる。

## インストール

```sh
go install github.com/edge2992/dw@latest
```

Goを使わない場合は、[Releases](https://github.com/edge2992/dw/releases/latest)
からOS/アーキテクチャ別のビルド済みバイナリ(linux/macOS/windows × amd64/arm64、
`checksums.txt` 付き)をダウンロードできる。

`dw --version` でインストール済みのバージョンを確認できる。

テンプレートはカテゴリごとに出し分けできる。作成するカテゴリに対し、次の順で
最初に見つかったものを使う:

1. `~/.config/discussion/templates/<category>.md` — カテゴリ専用
2. `~/.config/discussion/templates/default.md` — 全カテゴリ共通の既定
3. `~/.config/discussion/template.md` — 旧来の単一テンプレート（後方互換）
4. 組み込みの既定テンプレート（何も置かなくても動く）

いずれも `{{title}}` `{{category}}` `{{date}}` を置換する。例えば
`~/.config/discussion/templates/research.md` を置けば research カテゴリだけ
専用の雛形になる。

## 設計

- `internal/workspace` — スキャン / スラッグ化 / 作成 / テンプレート / 直前パスの永続化(純粋ロジック、テスト済み)
- `internal/tui` — bubbletea による単一あいまいリスト(ジャンプ + 作成 + カテゴリ選択 + ピン留め)
- `main.go` — 配線(`dw -` 即ジャンプ / scan → TUI → 選択パスを stdout 出力 → 直前パス記録)

## 開発

```sh
make fmt    # gofumpt + goimports (golangci-lint fmt)
make lint   # golangci-lint run
make test   # go test -race ./...
make        # 上記まとめて
```

- **Lint/Format**: golangci-lint v2(設定 `.golangci.yml`、standardセット + misspell/revive、formatterは gofumpt/goimports)
- **Hooks**: pre-commit framework(`.pre-commit-config.yaml`)。グローバル pre-commit hook が gitleaks の後に委譲するため `pre-commit install` は不要。導入: `uv tool install pre-commit`、`brew install golangci-lint`
- **CI**: GitHub Actions(`.github/workflows/ci.yml`)で build / test -race / golangci-lint を実行

## リリース

[Conventional Commits](https://www.conventionalcommits.org/) を解析して
[Release Please](https://github.com/googleapis/release-please) がバージョンを自動決定する。
main への push ごとに **release PR**(CHANGELOG付き)が更新され、その PR をマージすると
semver タグと GitHub Release が作られ、[GoReleaser](https://goreleaser.com/) が各OS/arch向けの
ビルド済みバイナリを Release に添付する(`.github/workflows/release.yml`)。`feat` は minor、
`fix` は patch を上げる。

## License

[MIT](LICENSE) © edge2992
