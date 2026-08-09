package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeState writes content as dir/STATE.md, creating dir first.
func writeState(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRenderState(t *testing.T) {
	got := renderState("Datadog Cost")
	if !containsLine(got, "# Datadog Cost") {
		t.Errorf("renderState title line missing, got:\n%s", got)
	}
	if !containsLine(got, unresolvedHeading) {
		t.Errorf("renderState is missing the unresolved-questions heading, got:\n%s", got)
	}
}

func TestReadState(t *testing.T) {
	cases := []struct {
		name           string
		content        string
		noFile         bool
		wantTitle      string
		wantUnresolved string
	}{
		{
			name: "normal",
			content: "# My Topic\n\n" +
				"## 前提\n\nsome fact\n\n" +
				unresolvedHeading + "\n\nFlex Logs pricing is TBD\nsecond line ignored\n",
			wantTitle:      "My Topic",
			wantUnresolved: "Flex Logs pricing is TBD",
		},
		{
			name:   "missing file",
			noFile: true,
		},
		{
			name: "legacy frontmatter",
			content: "---\ntitle: Old Style\ncategory: research\n---\n\n" +
				"# Old Style\n\n" + unresolvedHeading + "\n\nold open question\n",
			wantTitle:      "Old Style",
			wantUnresolved: "old open question",
		},
		{
			name:           "no H1 heading",
			content:        "some preamble\n\n" + unresolvedHeading + "\n\nquestion here\n",
			wantUnresolved: "question here",
		},
		{
			name:      "no unresolved section",
			content:   "# Topic Only\n\n## 前提\n\nfact\n",
			wantTitle: "Topic Only",
		},
		{
			name:      "unresolved section is only a comment",
			content:   "# Topic\n\n" + unresolvedHeading + "\n\n<!-- 先頭行が dw の一覧に出る -->\n",
			wantTitle: "Topic",
		},
		{
			name:      "unresolved section present but empty",
			content:   "# Topic\n\n" + unresolvedHeading + "\n\n## 次の見出し\n\nnope\n",
			wantTitle: "Topic",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if !c.noFile {
				writeState(t, dir, c.content)
			}
			got := readState(dir)
			if got.title != c.wantTitle {
				t.Errorf("title = %q, want %q", got.title, c.wantTitle)
			}
			if got.unresolved != c.wantUnresolved {
				t.Errorf("unresolved = %q, want %q", got.unresolved, c.wantUnresolved)
			}
		})
	}
}

// TestReadState_LongDocument guards against reintroducing a fixed line cap:
// a well-used STATE.md accumulates dated 前提/却下した案 entries over a
// topic's life, so "## 未決の問い" (and its first entry) must still be found
// even after the earlier sections have grown past what a small prefix scan
// would cover.
func TestReadState_LongDocument(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("# Long Topic\n\n## 前提\n\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "- 事実(2026-08-%02d): filler premise line %d\n", (i%28)+1, i)
	}
	b.WriteString("\n## 却下した案\n\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "- 案%d は採らない。理由は Y%d。\n", i, i)
	}
	b.WriteString("\n" + unresolvedHeading + "\n\nFlex Logs pricing is still TBD\n")
	writeState(t, dir, b.String())

	got := readState(dir)
	if got.title != "Long Topic" {
		t.Errorf("title = %q, want %q", got.title, "Long Topic")
	}
	if got.unresolved != "Flex Logs pricing is still TBD" {
		t.Errorf("unresolved = %q, want %q (the heading must not scroll out of a fixed scan window)", got.unresolved, "Flex Logs pricing is still TBD")
	}
}

func TestTSVRow(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, "# My Topic\n\n"+unresolvedHeading+"\n\nsome open question\n")
	p := Project{Name: "2026-08-08-my-topic", Topic: "my-topic", Date: "2026-08-08", Path: dir}

	row := p.TSVRow("")
	want := dir + "\t  My Topic\tsome open question\t2026-08-08-my-topic"
	if row != want {
		t.Errorf("TSVRow(\"\") = %q, want %q", row, want)
	}

	row = p.TSVRow(dir)
	want = dir + "\t* My Topic\tsome open question\t2026-08-08-my-topic"
	if row != want {
		t.Errorf("TSVRow(last=path) = %q, want %q", row, want)
	}

	row = p.TSVRow("/some/other/path")
	want = dir + "\t  My Topic\tsome open question\t2026-08-08-my-topic"
	if row != want {
		t.Errorf("TSVRow(last=other) = %q, want %q", row, want)
	}
}

// TestTSVRow_NameStaysSearchableWhenTitleDiverges guards the reason column 4
// exists: once a workspace's STATE.md title no longer resembles its directory
// name, the name column is the only thing left in fzf's search scope that
// still carries the slug and the date.
func TestTSVRow_NameStaysSearchableWhenTitleDiverges(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, "# Datadog コスト削減\n")
	p := Project{Name: "2026-08-08-datadog-cost", Topic: "datadog-cost", Date: "2026-08-08", Path: dir}

	cols := strings.Split(p.TSVRow(""), "\t")
	if len(cols) != 4 {
		t.Fatalf("TSVRow produced %d columns, want 4", len(cols))
	}
	if cols[3] != p.Name {
		t.Errorf("column 4 = %q, want the directory name %q", cols[3], p.Name)
	}
	if strings.Contains(cols[1], "datadog-cost") {
		t.Fatal("this test is vacuous unless the title column has diverged from the slug")
	}
}

func TestTSVRow_FallsBackToTopic(t *testing.T) {
	dir := t.TempDir() // no STATE.md at all
	p := Project{Name: "2026-08-08-my-topic", Topic: "my-topic", Date: "2026-08-08", Path: dir}
	row := p.TSVRow("")
	want := dir + "\t  my-topic\t\t2026-08-08-my-topic"
	if row != want {
		t.Errorf("TSVRow with no STATE.md = %q, want %q", row, want)
	}
}

func TestPinLast(t *testing.T) {
	a := Project{Path: "/a"}
	b := Project{Path: "/b"}
	c := Project{Path: "/c"}
	ps := []Project{a, b, c}

	got := PinLast(ps, "/c")
	want := []string{"/c", "/a", "/b"}
	assertPathOrder(t, got, want)

	got = PinLast(ps, "/a") // already first: unchanged
	assertPathOrder(t, got, []string{"/a", "/b", "/c"})

	got = PinLast(ps, "") // no pin: unchanged
	assertPathOrder(t, got, []string{"/a", "/b", "/c"})

	got = PinLast(ps, "/not-in-list")
	assertPathOrder(t, got, []string{"/a", "/b", "/c"})
}

func assertPathOrder(t *testing.T, got []Project, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d projects, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Path != w {
			t.Errorf("got[%d].Path = %q, want %q (full: %+v)", i, got[i].Path, w, got)
		}
	}
}
