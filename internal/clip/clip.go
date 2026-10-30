package clip

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
)

// mdConverter 启用了表格插件的转换器，保留 <table> 的结构。
var mdConverter = converter.NewConverter(
	converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
		table.NewTablePlugin(),
	),
)

// Result 是一次网页摘录的结果。
type Result struct {
	Title    string
	Markdown string
	URL      string
	Excerpt  string
}

// Extract 从 HTML 中提取正文并转换为 Markdown。
//
// 流程：
//  1. 解析 DOM
//  2. 移除脚本、样式、导航等噪声节点
//  3. 选出正文容器（article / main / role=main / 文本密度最高的容器）
//  4. 转 Markdown 并压缩空白
func Extract(rawHTML, pageURL, fallbackTitle string) (*Result, error) {
	if strings.TrimSpace(rawHTML) == "" {
		return nil, fmt.Errorf("网页内容为空")
	}
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("解析网页失败: %w", err)
	}

	// 收集标题
	title := extractTitle(doc)
	if title == "" {
		title = fallbackTitle
	}
	if title == "" && pageURL != "" {
		title = pageURL
	}

	stripNoise(doc)
	root := pickContentRoot(doc)
	if root == nil {
		return nil, fmt.Errorf("未能在网页中找到正文")
	}

	var htmlBuf bytes.Buffer
	if err := html.Render(&htmlBuf, root); err != nil {
		return nil, fmt.Errorf("渲染正文失败: %w", err)
	}
	mdBytes, err := mdConverter.ConvertNode(root)
	var md string
	if err != nil {
		// 回退到直接转换，确保不因表格等结构失败而丢内容
		fallback, ferr := mdConverter.ConvertString(htmlBuf.String())
		if ferr != nil {
			return nil, fmt.Errorf("转换为 Markdown 失败: %w", err)
		}
		md = fallback
	} else {
		md = string(mdBytes)
	}
	md = tidyMarkdown(md)
	if md == "" {
		return nil, fmt.Errorf("提取到的正文为空")
	}

	excerpt := firstParagraph(md)
	return &Result{
		Title:    strings.TrimSpace(title),
		Markdown: md,
		URL:      pageURL,
		Excerpt:  excerpt,
	}, nil
}

// stripNoise 删除脚本、样式、导航栏等非正文节点。
func stripNoise(doc *html.Node) {
	removeTags := map[string]bool{
		"script": true, "style": true, "noscript": true, "template": true,
		"iframe": true, "object": true, "embed": true, "canvas": true,
		"svg": true, "form": true, "input": true, "button": true,
		"select": true, "option": true, "textarea": true, "label": true,
		"nav": true, "aside": true, "footer": true, "header": true,
		"link": true, "meta": true, "video": true, "audio": true,
	}
	removeClass := regexp.MustCompile(`(?i)(^|[-_\s])(ad|ads|advert|advertisement|banner|breadcrumb|comment|comments|cookie|cookie-banner|disqus|footer|header|menu|nav|navigation|popup|modal|newsletter|overlay|paywall|promo|recommend|related|share|sidebar|social|subscribe|tag-list|toolbar|widget)([-_\s]|$)`)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.ElementNode {
				tag := strings.ToLower(c.Data)
				drop := removeTags[tag]
				if !drop {
					for _, attr := range c.Attr {
						if attr.Key == "role" && strings.EqualFold(attr.Val, "navigation") {
							drop = true
						}
						if attr.Key == "aria-hidden" && strings.EqualFold(attr.Val, "true") {
							drop = true
						}
						if attr.Key == "hidden" {
							drop = true
						}
						if attr.Key == "class" && removeClass.MatchString(attr.Val) {
							drop = true
						}
						if attr.Key == "id" && removeClass.MatchString(attr.Val) {
							drop = true
						}
					}
				}
				if drop {
					n.RemoveChild(c)
					c = next
					continue
				}
			}
			walk(c)
			c = next
		}
	}
	walk(doc)
}

// pickContentRoot 选出正文容器。
func pickContentRoot(doc *html.Node) *html.Node {
	// 优先使用语义化标签
	for _, tag := range []string{"article", "main"} {
		if n := findFirst(doc, tag); n != nil && textLength(n) > 200 {
			return n
		}
	}
	// role="main"
	for _, n := range collectElements(doc) {
		for _, attr := range n.Attr {
			if attr.Key == "role" && strings.EqualFold(attr.Val, "main") && textLength(n) > 200 {
				return n
			}
		}
	}
	// 退化为文本密度最高的 div/section
	var best *html.Node
	bestLen := 0
	for _, n := range collectElements(doc) {
		tag := strings.ToLower(n.Data)
		if tag != "div" && tag != "section" && tag != "td" {
			continue
		}
		l := textLength(n)
		if l > bestLen {
			best, bestLen = n, l
		}
	}
	if best != nil && bestLen > 100 {
		return best
	}
	return findFirst(doc, "body")
}

func findFirst(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, tag) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirst(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func collectElements(n *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func textLength(n *html.Node) int {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
			sb.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return len(strings.TrimSpace(sb.String()))
}

// extractTitle 依次尝试 title 标签、og:title、h1。
func extractTitle(doc *html.Node) string {
	if n := findFirst(doc, "title"); n != nil {
		if t := strings.TrimSpace(textContent(n)); t != "" {
			return t
		}
	}
	var og, h1 string
	for _, n := range collectElements(doc) {
		if n.Data == "meta" {
			for _, attr := range n.Attr {
				if attr.Key == "property" && strings.EqualFold(attr.Val, "og:title") {
					for _, a2 := range n.Attr {
						if a2.Key == "content" {
							og = strings.TrimSpace(a2.Val)
						}
					}
				}
			}
		}
		if n.Data == "h1" && h1 == "" {
			h1 = strings.TrimSpace(textContent(n))
		}
	}
	if og != "" {
		return og
	}
	return h1
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

var (
	blankLines    = regexp.MustCompile(`\n{3,}`)
	trailingSpace = regexp.MustCompile(`[ \t]+\n`)
	// 收紧列表：列表项之间的空行可以删除，
	// 但列表与上文段落之间的空行必须保留（Markdown 解析需要）。
	tightList = regexp.MustCompile(`(?m)([-+*] [^\n]*|\d+\. [^\n]*)\n\n([-+*] |\d+\. )`)
	// 修复粘连的列表项：上一项文本直接紧跟下一个列表符号。
	// 要求符号前一个字符非空白，避免误伤 "- 使用 -flag" 这类行内文本。
	gluedList = regexp.MustCompile(`(?m)(^|\n)((?:[-+*] |\d+\. )[^\n]*?[^\s])([-+*] |\d+\. )`)
)

// tidyMarkdown 压缩多余空行与行尾空格，并修复 HTML 转换后的结构问题。
func tidyMarkdown(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = strings.ReplaceAll(md, "\u200b", "")
	md = trailingSpace.ReplaceAllString(md, "\n")
	md = blankLines.ReplaceAllString(md, "\n\n")
	md = strings.TrimSpace(md)

	// 列表项之间的空行删掉，收紧为紧凑列表
	md = tightList.ReplaceAllString(md, "$1\n$2")

	// 修复粘连的列表项。一次重提不能完全拆开，需循环直至稳定。
	for i := 0; i < 20; i++ {
		fixed := gluedList.ReplaceAllString(md, "$1$2\n$3")
		if fixed == md {
			break
		}
		md = fixed
	}
	return md
}

func firstParagraph(md string) string {
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > 200 {
			return line[:200] + "…"
		}
		return line
	}
	return ""
}

// BuildNoteURL 构造来源链接。
func BuildNoteURL(pageURL string) string {
	if pageURL == "" {
		return ""
	}
	if _, err := url.Parse(pageURL); err != nil {
		return pageURL
	}
	return pageURL
}
