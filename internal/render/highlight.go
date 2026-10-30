package render

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// highlighter 是把 fenced code block 渲染为高亮 HTML 的 goldmark 扩展。
type highlighter struct{}

// Extend 在 goldmark 上注册 AST 转换器与自定义渲染器。
func (h *highlighter) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(parser.WithASTTransformers(util.Prioritized(&transformer{}, 100)))
	md.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(&codeBlockRenderer{}, 100)))
}

// transformer 遍历 AST，为代码块计算高亮 HTML 并挂到节点属性上。
type transformer struct{}

// Transform 遍历节点树，处理 FencedCodeBlock。
func (t *transformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		lang := string(fcb.Language(reader.Source()))
		code := codeText(fcb, reader)
		fcb.SetAttributeString("highlighted-html", HighlightCode(lang, code))
		return ast.WalkContinue, nil
	})
}

// codeText 提取代码块的原始文本。
func codeText(fcb *ast.FencedCodeBlock, reader text.Reader) string {
	var sb strings.Builder
	lines := fcb.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		sb.Write(seg.Value(reader.Source()))
	}
	return sb.String()
}

// codeBlockRenderer 覆盖默认的代码块渲染，输出高亮后的 HTML。
type codeBlockRenderer struct{}

// RegisterFuncs 注册各 AST 节点的渲染函数。
func (r *codeBlockRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
}

// renderFencedCodeBlock 渲染围栏代码块。
func (r *codeBlockRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	fcb := node.(*ast.FencedCodeBlock)
	html, _ := fcb.AttributeString("highlighted-html")
	out, _ := html.(string)
	if out == "" {
		// 未经过 transformer 时回退到默认渲染
		return ast.WalkContinue, nil
	}
	_, _ = w.Write([]byte(out))
	return ast.WalkSkipChildren, nil
}
