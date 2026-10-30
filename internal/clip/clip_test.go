package clip

import "testing"

func TestTidyMarkdownNoPanic(t *testing.T) {
	cases := []string{
		"",
		"# 标题\n\n正文 **加粗**。",
		"段落一\n\n\n\n段落二",
		"- 项目一\n\n- 项目二",
		"1. 第一\n\n2. 第二",
		"* 星号列表\n+ 加号列表\n- 减号列表",
		"```go\nfmt.Println(\"hi\")\n```",
		"行尾空格   \n下一行",
		"---",
		"含有 [链接](http://a.com) 与 ![图](http://b.png)",
	}
	for _, c := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("tidyMarkdown panic on %q: %v", c, r)
				}
			}()
			got := tidyMarkdown(c)
			if got == "" && c != "" {
				t.Errorf("tidyMarkdown(%q) 返回空", c)
			}
		}()
	}
}

func TestExtractBasic(t *testing.T) {
	htmlDoc := `<html><head><title>文章标题</title>
<meta property="og:title" content="OG 标题"></head>
<body>
<nav>导航栏内容</nav>
<header>页头</header>
<article>
  <h1>正文标题</h1>
  <p>这是<strong>重要</strong>段落。</p>
  <ul><li>要点一</li><li>要点二</li></ul>
  <blockquote>引用内容</blockquote>
  <pre><code>code block</code></pre>
</article>
<footer>页脚内容</footer>
<script>var x = 1;</script>
</body></html>`

	res, err := Extract(htmlDoc, "https://example.com/post", "")
	if err != nil {
		t.Fatalf("Extract 失败: %v", err)
	}
	if res.Title != "文章标题" {
		t.Errorf("标题错误: %q", res.Title)
	}
	md := res.Markdown
	for _, want := range []string{"正文标题", "重要", "要点一", "引用内容"} {
		if !contains(md, want) {
			t.Errorf("摘录缺少内容 %q\n实际:\n%s", want, md)
		}
	}
	for _, unwanted := range []string{"导航栏内容", "页脚内容", "var x"} {
		if contains(md, unwanted) {
			t.Errorf("摘录包含噪声 %q\n实际:\n%s", unwanted, md)
		}
	}
	if res.URL != "https://example.com/post" {
		t.Errorf("URL 错误: %q", res.URL)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestTidyMarkdownKeepsBoldAndSplitsLists(t *testing.T) {
	// 行内的 **bold** 不应被列表修复规则破坏
	in := "正文以 **channel** 为核心。\n- 项目一- 项目二"
	got := tidyMarkdown(in)
	if !contains(got, "**channel**") {
		t.Errorf("加粗文本被破坏:\n%s", got)
	}
	// 粘连的列表项应被拆开
	if !contains(got, "- 项目一\n- 项目二") {
		t.Errorf("列表项未拆分:\n%s", got)
	}
}

func TestExtractTablePreserved(t *testing.T) {
	doc := `<html><body><article>
	<table><thead><tr><th>机制</th><th>用途</th></tr></thead>
	<tbody><tr><td>mutex</td><td>保护共享状态</td></tr></tbody></table>
	</article></body></html>`
	res, err := Extract(doc, "https://x.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Markdown, "|") {
		t.Errorf("表格未转换为 Markdown 表格:\n%s", res.Markdown)
	}
	if !contains(res.Markdown, "mutex") {
		t.Errorf("表格内容丢失:\n%s", res.Markdown)
	}
}
