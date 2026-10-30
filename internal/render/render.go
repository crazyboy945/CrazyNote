package render

import (
	"bytes"
	stdhtml "html"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// md 是配置好的 Markdown 渲染器，支持 GFM 扩展（表格、任务列表、自动链接等），
// 代码块通过 chroma 做语法高亮。
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Footnote,
		&highlighter{},
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
		parser.WithAttribute(),
	),
	goldmark.WithRendererOptions(
		gmhtml.WithHardWraps(),
		gmhtml.WithXHTML(),
	),
)

// classFormatter 以 class 形式输出高亮标记（颜色由配套 CSS 决定）。
var classFormatter = html.New(
	html.WithClasses(true),
	html.ClassPrefix("cn"),
)

// Markdown 把 Markdown 文本渲染为 HTML。
//
// 代码块会通过 chroma 做语法高亮，输出带 .chroma class 的 HTML，
// 具体颜色由配套的 CSS 主题决定（见 ThemeCSS）。
func Markdown(src string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return stdhtml.EscapeString(src)
	}
	return buf.String()
}

// HighlightCode 对单个代码块做语法高亮，返回带 class 的 HTML 片段。
//
// lang 为语言名（可为空），code 为代码内容。
// 无法识别语言时回退为纯文本，输出仍带 chroma 结构以便统一样式。
func HighlightCode(lang, code string) string {
	lexer := lookupLexer(lang)
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return stdhtml.EscapeString(code)
	}
	var buf bytes.Buffer
	if err := classFormatter.Format(&buf, styles.Get("github"), iterator); err != nil {
		return stdhtml.EscapeString(code)
	}
	return buf.String()
}

// lookupLexer 按语言名查找词法分析器。
//
// 依次尝试：完整名（含别名）、大小写不敏感匹配。
func lookupLexer(lang string) chroma.Lexer {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return lexers.Fallback
	}
	if l := lexers.Get(lang); l != nil {
		return l
	}
	// 忽略大小写再试一次（如 "JavaScript" vs "javascript"）
	for _, l := range lexers.GlobalLexerRegistry.Lexers {
		if strings.EqualFold(l.Config().Name, lang) {
			return l
		}
		for _, alias := range l.Config().Aliases {
			if strings.EqualFold(alias, lang) {
				return l
			}
		}
	}
	return lexers.Fallback
}

// ThemeCSS 返回指定主题的高亮样式表。
//
// theme 取 "github"（亮色）或 "github-dark"（暗色）。
// 生成的 CSS 以 .chroma 前缀的 class 选择器组织，页面引入即可生效。
func ThemeCSS(theme string) string {
	style := styles.Get("github")
	if theme == "github-dark" || theme == "dark" {
		style = styles.Get("github-dark")
	}
	if style == nil {
		style = styles.Get("github")
	}
	var buf bytes.Buffer
	if err := classFormatter.WriteCSS(&buf, style); err != nil {
		return ""
	}
	return buf.String()
}

// SupportedLanguages 返回已支持的语言名列表，用于前端提示。
func SupportedLanguages() []string {
	names := lexers.GlobalLexerRegistry.Lexers
	out := make([]string, 0, len(names))
	for _, l := range names {
		cfg := l.Config()
		if cfg == nil {
			continue
		}
		out = append(out, cfg.Name)
	}
	return out
}
