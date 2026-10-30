package render

import (
	"strings"
	"testing"
)

func TestHighlightCodeProducesClasses(t *testing.T) {
	code := `func main() {
	fmt.Println("hello")
}`
	html := HighlightCode("go", code)
	if !strings.Contains(html, "chroma") {
		t.Errorf("高亮输出缺少 chroma class:\n%s", html)
	}
	if !strings.Contains(html, "<span") {
		t.Errorf("高亮输出没有任何 span 标记:\n%s", html)
	}
	// 代码内容本身应被保留
	if !strings.Contains(html, "Println") {
		t.Errorf("高亮丢失代码内容:\n%s", html)
	}
}

func TestHighlightCodeUnknownLanguage(t *testing.T) {
	// 未知语言应回退为纯文本，不报错
	html := HighlightCode("totally-not-a-language", "some text here")
	if html == "" {
		t.Error("未知语言返回空结果")
	}
	if !strings.Contains(html, "some text here") {
		t.Errorf("未知语言丢失内容:\n%s", html)
	}
}

func TestHighlightCodeEmptyLanguage(t *testing.T) {
	html := HighlightCode("", "plain content")
	if !strings.Contains(html, "plain content") {
		t.Errorf("空语言丢失内容:\n%s", html)
	}
}

func TestHighlightCodeCaseInsensitive(t *testing.T) {
	// 大小写不敏感的语言名也应命中
	html := HighlightCode("JavaScript", "const x = 1;")
	if !strings.Contains(html, "cn") {
		t.Errorf("大小写不敏感匹配失败:\n%s", html)
	}
}

func TestMarkdownRendersCodeBlock(t *testing.T) {
	src := "# 标题\n\n```go\nfunc main() {}\n```\n"
	html := Markdown(src)
	if !strings.Contains(html, "<h1") {
		t.Errorf("标题未渲染:\n%s", html)
	}
	// 代码块经高亮后以 chroma class 的 span 输出
	if !strings.Contains(html, "chroma") {
		t.Errorf("代码块未做语法高亮:\n%s", html)
	}
	if !strings.Contains(html, "main") {
		t.Errorf("代码内容丢失:\n%s", html)
	}
}

func TestMarkdownCodeBlockHasLanguageClass(t *testing.T) {
	src := "```python\nprint('hi')\n```\n"
	html := Markdown(src)
	// chroma 会为不同语言生成不同的 token class，如 cnnf（函数名）
	if !strings.Contains(html, "cn") {
		t.Errorf("python 代码块未高亮:\n%s", html)
	}
}

func TestMarkdownInlineCodeUnaffected(t *testing.T) {
	// 行内代码不应被高亮处理破坏
	src := "这是 `inline` 代码"
	html := Markdown(src)
	if !strings.Contains(html, "<code") {
		t.Errorf("行内代码未渲染:\n%s", html)
	}
}

func TestMarkdownMultipleCodeBlocks(t *testing.T) {
	src := "```go\na := 1\n```\n\n中间文字\n\n```js\nlet b = 2\n```\n"
	html := Markdown(src)
	if strings.Count(html, "cn") < 4 {
		t.Errorf("多个代码块未全部高亮:\n%s", html)
	}
	if !strings.Contains(html, "中间文字") {
		t.Errorf("代码块之间的正文丢失:\n%s", html)
	}
}

func TestThemeCSSNotEmpty(t *testing.T) {
	for _, theme := range []string{"github", "github-dark", "dark", ""} {
		css := ThemeCSS(theme)
		if strings.TrimSpace(css) == "" {
			t.Errorf("主题 %q 的 CSS 为空", theme)
		}
		if !strings.Contains(css, ".cn") {
			t.Errorf("主题 %q 的 CSS 缺少高亮选择器", theme)
		}
	}
}

func TestThemeCSSDarkDiffersFromLight(t *testing.T) {
	light := ThemeCSS("github")
	dark := ThemeCSS("github-dark")
	if light == dark {
		t.Error("亮色与暗色主题 CSS 相同")
	}
}

func TestSupportedLanguagesNotEmpty(t *testing.T) {
	langs := SupportedLanguages()
	if len(langs) < 20 {
		t.Errorf("支持的语言过少: %d", len(langs))
	}
}
