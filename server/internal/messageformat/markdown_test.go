package messageformat

import "testing"

func TestMarkdownPlainTextPreservesLegacySummaryLayout(t *testing.T) {
	content := "| 姓名 | 年龄 |\n| --- | ---: |\n| 张三 | 18 |\n\n这个计划~~废弃~~保留\n\n---\n\n继续推进"
	got, err := MarkdownPlainText(content)
	if err != nil {
		t.Fatalf("parse markdown: %v", err)
	}
	want := "姓名 年龄\n张三 18\n这个计划废弃保留\n继续推进"
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}
