package workspace

import (
	"os"
	"path/filepath"
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

func TestTSVRow(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, "# My Topic\n\n"+unresolvedHeading+"\n\nsome open question\n")
	p := Project{Name: "2026-08-08-my-topic", Topic: "my-topic", Date: "2026-08-08", Path: dir}

	row := p.TSVRow("")
	want := dir + "\t  My Topic\tsome open question"
	if row != want {
		t.Errorf("TSVRow(\"\") = %q, want %q", row, want)
	}

	row = p.TSVRow(dir)
	want = dir + "\t* My Topic\tsome open question"
	if row != want {
		t.Errorf("TSVRow(last=path) = %q, want %q", row, want)
	}

	row = p.TSVRow("/some/other/path")
	want = dir + "\t  My Topic\tsome open question"
	if row != want {
		t.Errorf("TSVRow(last=other) = %q, want %q", row, want)
	}
}

func TestTSVRow_FallsBackToTopic(t *testing.T) {
	dir := t.TempDir() // no STATE.md at all
	p := Project{Name: "2026-08-08-my-topic", Topic: "my-topic", Date: "2026-08-08", Path: dir}
	row := p.TSVRow("")
	want := dir + "\t  my-topic\t"
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
