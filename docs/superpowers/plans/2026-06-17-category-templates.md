# カテゴリ別テンプレート Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** カテゴリごとのテンプレートを規約ベース（設定ファイルなし）で出し分けられるようにする。

**Architecture:** テンプレート解決を起動時の1回読みから「カテゴリ確定後」に移す。`workspace.ResolveTemplate(category)` が `~/.config/discussion/templates/<category>.md` → `templates/default.md` → 既存 `template.md`（後方互換） → 組み込み `DefaultTemplate` の順で解決する。TUI の `New` から `tmpl` 引数を外し、`onEnter` のカテゴリ確定時に解決する。

**Tech Stack:** Go 1.x, bubbletea（TUI）, 標準ライブラリ `os`/`path/filepath` のみ（新規依存なし）。

参照: 設計 spec `docs/superpowers/specs/2026-06-17-category-templates-design.md`

---

## File Structure

- `internal/workspace/template.go` — `ResolveTemplate` / 内部 `resolveTemplate` を追加。未使用になる `LoadTemplate` を削除（`internal` パッケージなので外部影響なし）。`TemplatePath`/`RenderTemplate`/`DefaultTemplate` は維持。
- `internal/workspace/template_test.go` — **新規**。`resolveTemplate` の解決順テスト。
- `internal/tui/tui.go` — `Model.tmpl` フィールドと `New` の `tmpl` 引数を削除。`onEnter` で `workspace.ResolveTemplate(r.label)` を解決して `Create` に渡す。
- `internal/tui/tui_test.go` — 全 `New(...)` 呼び出し（9箇所）を新シグネチャに追従。カテゴリ別解決のE2Eテストを追加。
- `main.go` — 起動時の `LoadTemplate`/`TemplatePath` 配線を削除し、`tui.New` を新シグネチャへ。
- `README.md` — カテゴリ別テンプレートの規約を追記。

---

## Task 1: テンプレート解決ロジック（workspace層）

**Files:**
- Test: `internal/workspace/template_test.go`（新規）
- Modify: `internal/workspace/template.go`

- [ ] **Step 1: 失敗するテストを書く**

新規ファイル `internal/workspace/template_test.go`:

```go
package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTemplate(t *testing.T) {
	base := t.TempDir()
	tmplDir := filepath.Join(base, "templates")
	legacy := filepath.Join(base, "template.md")

	// 4. ディスクに何も無い → 組み込み DefaultTemplate
	if got := resolveTemplate(tmplDir, legacy, "research"); got != DefaultTemplate {
		t.Fatalf("empty dir should fall back to DefaultTemplate")
	}

	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 3. legacy template.md のみ
	if err := os.WriteFile(legacy, []byte("LEGACY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveTemplate(tmplDir, legacy, "research"); got != "LEGACY" {
		t.Fatalf("should use legacy path, got %q", got)
	}
	// 2. templates/default.md が legacy を上書き
	if err := os.WriteFile(filepath.Join(tmplDir, "default.md"), []byte("DEFAULT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveTemplate(tmplDir, legacy, "research"); got != "DEFAULT" {
		t.Fatalf("should use templates/default.md, got %q", got)
	}
	// 1. templates/<category>.md が最優先
	if err := os.WriteFile(filepath.Join(tmplDir, "research.md"), []byte("RESEARCH"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveTemplate(tmplDir, legacy, "research"); got != "RESEARCH" {
		t.Fatalf("should use templates/research.md, got %q", got)
	}
	// 専用ファイルの無いカテゴリは default に落ちる
	if got := resolveTemplate(tmplDir, legacy, "incident"); got != "DEFAULT" {
		t.Fatalf("uncovered category should use default, got %q", got)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/workspace/ -run TestResolveTemplate -v`
Expected: コンパイルエラー `undefined: resolveTemplate`

- [ ] **Step 3: 最小実装を書く**

`internal/workspace/template.go` に追加（`LoadTemplate` 関数は削除し、以下で置き換え）。`os`/`path/filepath`/`strings`/`bufio` は既存 import 済み。

```go
// ResolveTemplate picks the template for a category using the convention-based
// search order, falling back to the built-in DefaultTemplate.
func ResolveTemplate(category string) string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "discussion", "templates")
	return resolveTemplate(dir, TemplatePath(), category)
}

// resolveTemplate implements the search order with injectable paths for testing:
//   1. <dir>/<category>.md  (category-specific)
//   2. <dir>/default.md     (shared default)
//   3. legacyPath           (~/.config/discussion/template.md, backward compat)
//   4. DefaultTemplate      (built-in)
func resolveTemplate(dir, legacyPath, category string) string {
	for _, p := range []string{
		filepath.Join(dir, category+".md"),
		filepath.Join(dir, "default.md"),
		legacyPath,
	} {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return DefaultTemplate
}
```

既存の `LoadTemplate` 関数（`template.go:34-41`）を削除する:

```go
// LoadTemplate reads the template file, falling back to DefaultTemplate.
func LoadTemplate(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return DefaultTemplate
	}
	return string(b)
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/workspace/ -run TestResolveTemplate -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/workspace/template.go internal/workspace/template_test.go
git commit -m "feat: add convention-based category template resolution"
```

---

## Task 2: TUI/main を解決時点の変更に追従

`New` から `tmpl` を外し、カテゴリ確定時に `ResolveTemplate` で解決する配線変更。振る舞いは「カテゴリ別テンプレートがあれば使う」に変わるが、無ければ従来どおり `DefaultTemplate`。コンパイルを通し既存テストを緑に保つリファクタ。

**Files:**
- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/tui_test.go`
- Modify: `main.go`

- [ ] **Step 1: `tui.go` の Model と New を変更**

`Model` 構造体（`tui.go:50-67`）の `tmpl string` フィールドを削除する。該当行:

```go
	root       string
	tmpl       string
	now        time.Time
```

を次に:

```go
	root       string
	now        time.Time
```

`New`（`tui.go:72-81`）を次に置き換える:

```go
// New builds the initial model. lastPath, when it matches a project, is pinned
// to the top of the browse list so the most common move — resuming the previous
// workspace — is one Enter away; pass "" to disable pinning.
func New(root string, now time.Time, projects []workspace.Project, lastPath string) Model {
	return Model{
		root:       root,
		now:        now,
		projects:   pinLast(projects, lastPath),
		categories: workspace.Categories(projects),
		mode:       modeBrowse,
	}
}
```

- [ ] **Step 2: `onEnter` でカテゴリ別テンプレートを解決**

`onEnter`（`tui.go:240`）の `Create` 呼び出しを置き換える。現状:

```go
	// modeCategory: r.label is the category (existing or new)
	p, err := workspace.Create(m.root, r.label, m.pendingTopic, m.now, m.tmpl)
```

を次に:

```go
	// modeCategory: r.label is the category (existing or new). Resolve the
	// template now that the category is known so per-category templates apply.
	tmpl := workspace.ResolveTemplate(r.label)
	p, err := workspace.Create(m.root, r.label, m.pendingTopic, m.now, tmpl)
```

- [ ] **Step 3: `main.go` の配線を更新**

`main.go:37` の行を削除:

```go
	tmpl := workspace.LoadTemplate(workspace.TemplatePath())
```

`main.go:39` を次に置き換える:

```go
	model := tui.New(root, time.Now(), projects, workspace.LastPath())
```

- [ ] **Step 4: `tui_test.go` の全 New 呼び出しを追従**

`tui_test.go` 内の `New(...)` は9箇所。すべて第2引数の `workspace.DefaultTemplate,` を削除する。具体的な置換:

`tui_test.go:51` `m := New("/d", workspace.DefaultTemplate, time.Now(), projects, "")`
→ `m := New("/d", time.Now(), projects, "")`

`tui_test.go:65` `m := New("/d", workspace.DefaultTemplate, time.Now(), projects, "")`
→ `m := New("/d", time.Now(), projects, "")`

`tui_test.go:77` `m := New(root, workspace.DefaultTemplate, now, nil, "")`
→ `m := New(root, now, nil, "")`

`tui_test.go:107` `m := New(root, workspace.DefaultTemplate, now, nil, "")`
→ `m := New(root, now, nil, "")`

`tui_test.go:137` `m := New(root, workspace.DefaultTemplate, now, nil, "")`
→ `m := New(root, now, nil, "")`

`tui_test.go:167-168`
```go
	m := New("/d", workspace.DefaultTemplate, time.Now(),
		[]workspace.Project{{Path: "/d/x", Name: "n"}}, "")
```
→
```go
	m := New("/d", time.Now(),
		[]workspace.Project{{Path: "/d/x", Name: "n"}}, "")
```

`tui_test.go:178` `m := New(root, workspace.DefaultTemplate, now, nil, "")`
→ `m := New(root, now, nil, "")`

`tui_test.go:210` `m := New("/d", workspace.DefaultTemplate, time.Now(), projects, "/d/b/2026-06-10-older")`
→ `m := New("/d", time.Now(), projects, "/d/b/2026-06-10-older")`

`tui_test.go:223` `m := New("/d", workspace.DefaultTemplate, time.Now(), projects, "/d/gone")`
→ `m := New("/d", time.Now(), projects, "/d/gone")`

- [ ] **Step 5: 作成系テストをホスト環境から分離**

`onEnter` が実 `~/.config/discussion` を読まないよう、実際に `Create` を呼ぶ2テストの先頭に `HOME` 差し替えを追加する。

`TestCreateFlow`（`tui_test.go:74-77`）の `root := t.TempDir()` の直後に1行追加:

```go
func TestCreateFlow(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	m := New(root, now, nil, "")
```

`TestNewCategoryCreation`（`tui_test.go:104-107`）も同様に `root := t.TempDir()` の直後へ:

```go
func TestNewCategoryCreation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	m := New(root, now, nil, "")
```

（`TestEscFromCategoryReturnsToBrowse` は `Create` まで到達しないため変更不要。）

- [ ] **Step 6: ビルドと全テストを確認**

Run: `go build ./... && go test ./...`
Expected: すべて PASS（コンパイル成功、既存テスト緑）

- [ ] **Step 7: コミット**

```bash
git add internal/tui/tui.go internal/tui/tui_test.go main.go
git commit -m "refactor: resolve template at category-confirm time"
```

---

## Task 3: カテゴリ別解決のE2Eテスト（TUI経由）

カテゴリ別テンプレートが TUI 作成フロー経由で実際に反映されることを検証する。

**Files:**
- Modify: `internal/tui/tui_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`tui_test.go` 末尾に追加（import の `os`/`path/filepath`/`time` は既存）:

```go
func TestCreateUsesCategoryTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tmplDir := filepath.Join(home, ".config", "discussion", "templates")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "research.md"), []byte("CATEGORY BODY"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	m := New(root, now, nil, "")

	// browse: topic を入力し create 行へ
	m = typeStr(m, "topic")
	rows := m.rows()
	for i := 0; i < len(rows)-1; i++ {
		m = send(m, "down")
	}
	m = send(m, "enter") // -> category mode
	// 先頭の既定カテゴリ research を確定
	m = send(m, "enter")

	b, err := os.ReadFile(filepath.Join(m.Result, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "CATEGORY BODY" {
		t.Fatalf("README = %q, want category template body", string(b))
	}
}
```

- [ ] **Step 2: テストが通ることを確認**

Run: `go test ./internal/tui/ -run TestCreateUsesCategoryTemplate -v`
Expected: PASS（Task 2 で `onEnter` が `ResolveTemplate` を使うため）

- [ ] **Step 3: コミット**

```bash
git add internal/tui/tui_test.go
git commit -m "test: cover category template selection end-to-end"
```

---

## Task 4: README にカテゴリ別テンプレートを記載

**Files:**
- Modify: `README.md`

- [ ] **Step 1: 「インストール」節のテンプレート説明を更新**

`README.md` 現状のテンプレート記述:

```
テンプレートは `~/.config/discussion/template.md` があればそれを使い、
無ければ組み込みの既定テンプレートを使う。`{{title}}` `{{category}}` `{{date}}`
を置換する。
```

を次に置き換える:

```
テンプレートはカテゴリごとに出し分けできる。作成するカテゴリに対し、次の順で
最初に見つかったものを使う:

1. `~/.config/discussion/templates/<category>.md` — カテゴリ専用
2. `~/.config/discussion/templates/default.md` — 全カテゴリ共通の既定
3. `~/.config/discussion/template.md` — 旧来の単一テンプレート（後方互換）
4. 組み込みの既定テンプレート（何も置かなくても動く）

いずれも `{{title}}` `{{category}}` `{{date}}` を置換する。例えば
`~/.config/discussion/templates/research.md` を置けば research カテゴリだけ
専用の雛形になる。
```

- [ ] **Step 2: コミット**

```bash
git add README.md
git commit -m "docs: document category templates"
```

---

## Task 5: 最終検証

- [ ] **Step 1: race 付き全テスト**

Run: `go test -race ./...`
Expected: すべて ok

- [ ] **Step 2: lint**

Run: `make lint`
Expected: 0 issues（未使用 import が無いこと。`tui_test.go` が `workspace` を引き続き使用していること＝`workspace.Project`/`workspace.Slugify`/`workspace.DefaultTemplate` 参照が残るため import は維持される）

- [ ] **Step 3: 手動E2E確認**

```bash
mkdir -p ~/.config/discussion/templates
printf 'RESEARCH TEMPLATE for {{title}}\n' > ~/.config/discussion/templates/research.md
DISCUSSION_ROOT=$(mktemp -d) go run . </dev/null   # TUIは手動操作。research に作成して内容確認
```
Expected: research カテゴリで作成した README が `RESEARCH TEMPLATE for <topic>` になる。`templates/` を消すと組み込みデフォルトに戻る。
（確認後 `rm ~/.config/discussion/templates/research.md` で後片付け。）

---

## Notes
- 新規依存なし。設定ファイルパーサ不要。
- `internal` パッケージのため `LoadTemplate` 削除は外部破壊なし。
- カテゴリ名は `Slugify` 済みでファイル名として安全。
