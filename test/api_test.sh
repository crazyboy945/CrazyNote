#!/usr/bin/env bash
# CrazyNote 接口自测脚本
#
# 可用环境变量：
#   CRAZYNOTE_BASE   服务地址，默认 http://127.0.0.1:18899
#   CRAZYNOTE_NOTES  笔记目录（须与服务 -data 一致），默认 $CRAZYNOTE_WORK/notes
#   CRAZYNOTE_LOG    服务日志路径，用于读取初始口令
#   CRAZYNOTE_WORK   测试工作目录（存放 cookie 等临时文件）
BASE="${CRAZYNOTE_BASE:-http://127.0.0.1:18899}"
WORK="${CRAZYNOTE_WORK:-/c/Users/Administrator/crazynote-test}"
mkdir -p "$WORK"
# 笔记目录由服务的 -data 参数指定
NOTES="${CRAZYNOTE_NOTES:-$WORK/notes}"
PASS=0; FAIL=0
ok()   { echo "  [PASS] $1"; PASS=$((PASS+1)); }
bad()  { echo "  [FAIL] $1 -- $2"; FAIL=$((FAIL+1)); }
chk()  { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1" "期望=$3 实际=$2"; fi }

echo "=== 1. 健康检查 ==="
R=$(curl -s --noproxy "*" $BASE/api/health)
echo "$R" | grep -q '"status":"ok"' && ok "health" || bad "health" "$R"

echo "=== 2. 未认证应返回 401 ==="
CODE=$(curl -s --noproxy "*" -o /dev/null -w "%{http_code}" $BASE/api/categories)
chk "未认证返回401" "$CODE" "401"

echo "=== 3. 登录 ==="
LOG="${CRAZYNOTE_LOG:-$WORK/server.log}"
if [ ! -f "$LOG" ]; then
  # Git Bash 的 /tmp 与 Windows exe 的 Temp 路径表示不一致，尝试另一种
  WIN_TMP="/c/Users/Administrator/AppData/Local/Temp/workbuddy-update-x64${WORK}"
  if [ -f "$WIN_TMP/server.log" ]; then
    LOG="$WIN_TMP/server.log"
  fi
fi
PWD=$(grep -oP '访问口令: \K\S+' "$LOG" | tail -1)
echo "  口令: $PWD"

# 优先从服务日志读取真实笔记目录，避免 Windows 与 Git Bash 的路径表示差异
if [ -z "${CRAZYNOTE_NOTES:-}" ]; then
  NOTES_WIN=$(grep -oP '笔记目录: \K.*' "$LOG" | tail -1)
  if [ -n "$NOTES_WIN" ]; then
    # Windows 路径 C:\a\b -> /c/a/b
    NOTES=$(printf '%s' "$NOTES_WIN" | sed -E 's#^([A-Za-z]):[\\/](.*)$#/\L\1/\2#; s#\\#/#g')
    # Windows 的 8.3 短路径（ADMINI~1）与长路径指向同一目录，
    # 短路径在某些环境下大小写不一致会导致断言失败，这里做一次存在性校验
    if [ ! -d "$NOTES" ]; then
      NOTES_DIR_WIN=$(grep -oP '笔记目录: \K.*' "$LOG" | tail -1)
      NOTES=$(printf '%s' "$NOTES_DIR_WIN" | sed -E 's#^([A-Za-z]):[\\/](.*)$#/\L\1/\2#; s#\\#/#g')
    fi
  fi
fi
echo "  笔记目录: $NOTES"

R=$(curl -s --noproxy "*" -c "$WORK"/cookie.txt -X POST $BASE/api/login \
  -H 'Content-Type: application/json' -d "{\"password\":\"$PWD\"}")
echo "$R" | grep -q '"user":"admin"' && ok "登录成功" || bad "登录" "$R"

echo "=== 4. 会话信息 ==="
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt $BASE/api/session)
echo "$R" | grep -q '"is_admin":true' && ok "session" || bad "session" "$R"

echo "=== 4.5 清理旧分类（保证测试幂等）==="
curl -s --noproxy "*" -b "$WORK"/cookie.txt $BASE/api/categories \
  | python -c "
import json,sys
d=json.load(sys.stdin)
print(' '.join(c['name'] for c in d.get('categories',[])))
" > "$WORK"/cats.txt
for c in $(cat "$WORK"/cats.txt); do
  curl -s --noproxy "*" -b "$WORK"/cookie.txt -X DELETE "$BASE/api/categories/$c" > /dev/null
done
echo "  已清理: $(cat "$WORK"/cats.txt)"

echo "=== 5. 创建分类 ==="
for c in 技术 读书 摘录; do
  R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt -X POST $BASE/api/categories \
    -H 'Content-Type: application/json' -d "{\"name\":\"$c\"}")
  echo "$R" | grep -q '"ok":true' && ok "创建分类 $c" || bad "创建分类 $c" "$R"
done

echo "=== 6. 校验磁盘目录结构 ==="
[ -d "${NOTES}/技术" ] && ok "分类=子目录 技术" || bad "分类目录" "未创建"
[ -d "${NOTES}/读书" ] && ok "分类=子目录 读书" || bad "分类目录" "未创建"

echo "=== 7. 新建笔记 ==="
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt -X POST $BASE/api/notes \
  -H 'Content-Type: application/json' \
  -d '{"category":"技术","title":"Go 交叉编译笔记","body":"# 标题\n\n正文 **加粗**。\n\n- 项目一\n- 项目二\n","tags":["go","编译"]}')
echo "$R" | grep -q '"title":"Go 交叉编译笔记"' && ok "创建笔记" || bad "创建笔记" "$R"
FILE=$(echo "$R" | grep -oP '"file":"\K[^"]+')
echo "  文件名: $FILE"

echo "=== 8. 校验 .md 文件内容 ==="
MD="${NOTES}/技术/$FILE"
[ -f "$MD" ] && ok "Markdown 文件落盘" || bad "文件落盘" "$MD"
grep -q "^title: Go 交叉编译笔记" "$MD" && ok "frontmatter title" || bad "frontmatter title" ""
grep -q "^---" "$MD" && ok "frontmatter 分隔符" || bad "frontmatter 分隔" ""
grep -q "^# 标题" "$MD" && ok "正文保留" || bad "正文" ""

echo "=== 9. 列表与计数 ==="
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt "$BASE/api/notes?category=%E6%8A%80%E6%9C%AF")
COUNT=$(echo "$R" | grep -oP '"notes":\[\K' | wc -l)
echo "$R" | grep -q 'Go 交叉编译笔记' && ok "列表包含笔记" || bad "列表" "$R"
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt $BASE/api/categories)
echo "$R" | grep -q '"name":"技术","count":1' && ok "分类计数正确" || bad "分类计数" "$R"

echo "=== 10. 搜索 ==="
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt "$BASE/api/search?q=%E5%8A%A0%E7%B2%97")
echo "$R" | grep -q 'Go 交叉编译笔记' && ok "搜索命中正文" || bad "搜索" "$R"
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt "$BASE/api/search?q=go")
echo "$R" | grep -q 'Go 交叉编译笔记' && ok "搜索命中标签" || bad "搜索标签" "$R"

echo "=== 11. 网页摘录（HTML→Markdown）==="
HTML='<html><head><title>测试文章标题</title></head><body><nav>导航栏</nav><article><h1>正文标题</h1><p>这是<strong>重要</strong>段落。</p><ul><li>要点一</li><li>要点二</li></ul></article><footer>页脚</footer></body></html>'
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt -X POST $BASE/api/clip \
  -H 'Content-Type: application/json' \
  -d "{\"category\":\"摘录\",\"title\":\"测试文章标题\",\"url\":\"https://example.com/a\",\"html\":$(echo "$HTML" | python -c 'import json,sys;print(json.dumps(sys.stdin.read()))')}")
echo "$R" | grep -q '"ok":true' && ok "摘录成功" || bad "摘录" "$R"
CLIPFILE=$(echo "$R" | grep -oP '"file":"\K[^"]+')
echo "  摘录文件: $CLIPFILE"
CM="${NOTES}/摘录/$CLIPFILE"
grep -q "正文标题" "$CM" && ok "摘录保留标题" || bad "摘录标题" "$(cat $CM 2>/dev/null | head -5)"
grep -q "重要" "$CM" && ok "摘录保留加粗" || bad "摘录加粗" ""
grep -q "要点一" "$CM" && ok "摘录保留列表" || bad "摘录列表" ""
grep -q "example.com" "$CM" && ok "摘录保留来源链接" || bad "摘录来源" ""
grep -q "导航栏" "$CM" && bad "导航栏被剔除" "未剔除" || ok "导航栏已剔除"

echo "=== 12. API Token 免登录摘录 ==="
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt -X POST $BASE/api/tokens -H 'Content-Type: application/json' -d '{}')
TOKEN=$(echo "$R" | grep -oP '"token":"\K[^"]+')
[ -n "$TOKEN" ] && ok "生成 Token" || bad "生成 Token" "$R"
R=$(curl -s --noproxy "*" -X POST "$BASE/api/clip?token=$TOKEN" -H 'Content-Type: application/json' \
  -d '{"category":"摘录","title":"令牌测试","body":"通过令牌写入的内容"}')
echo "$R" | grep -q '"ok":true' && ok "Token 免登录摘录" || bad "Token 摘录" "$R"
R=$(curl -s --noproxy "*" -o /dev/null -w "%{http_code}" -X POST "$BASE/api/clip?token=invalid" \
  -H 'Content-Type: application/json' -d '{"body":"x"}')
chk "无效Token返回401" "$R" "401"

echo "=== 13. 保存笔记（更新）==="
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt -X PUT \
  "$BASE/api/notes/%E6%8A%80%E6%9C%AF/$(python -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$FILE")" \
  -H 'Content-Type: application/json' -d '{"title":"已更新的标题","body":"更新后的正文","tags":["go"]}')
echo "$R" | grep -q '"title":"已更新的标题"' && ok "更新笔记" || bad "更新笔记" "$R"
NEWFILE=$(echo "$R" | grep -oP '"file":"\K[^"]+')
[ "$NEWFILE" != "$FILE" ] && ok "标题变化触发文件重命名 ($FILE -> $NEWFILE)" || bad "文件重命名" "未变化"

echo "=== 14. 移动笔记到其他分类 ==="
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt -X POST \
  "$BASE/api/notes/%E6%8A%80%E6%9C%AF/$(python -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$NEWFILE")/move" \
  -H 'Content-Type: application/json' -d '{"target":"读书"}')
echo "$R" | grep -q '"category":"读书"' && ok "移动分类" || bad "移动分类" "$R"
[ -f "${NOTES}/读书/已更新的标题.md" ] && ok "文件已移到目标目录" || bad "移动文件" ""

echo "=== 15. 路径穿越防护 ==="
CODE=$(curl -s --noproxy "*" -o /dev/null -w "%{http_code}" -b "$WORK"/cookie.txt \
  "$BASE/api/notes/..%2F..%2Fetc/..%2Fpasswd")
[ "$CODE" = "400" ] || [ "$CODE" = "404" ] && ok "穿越被拒绝 ($CODE)" || bad "穿越防护" "$CODE"
CODE=$(curl -s --noproxy "*" -o /dev/null -w "%{http_code}" -b "$WORK"/cookie.txt \
  "$BASE/api/notes/技术/..%2F..%2F..%2Fetc%2Fpasswd.md")
[ "$CODE" = "400" ] || [ "$CODE" = "404" ] && ok "文件穿越被拒绝 ($CODE)" || bad "文件穿越" "$CODE"

echo "=== 16. 标签聚合 ==="
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt $BASE/api/tags)
echo "$R" | grep -q '"go"' && ok "标签聚合" || bad "标签聚合" "$R"

echo "=== 17. 重命名分类 ==="
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt -X PUT "$BASE/api/categories/%E8%AF%BB%E4%B9%A6" \
  -H 'Content-Type: application/json' -d '{"name":"阅读"}')
echo "$R" | grep -q '"ok":true' && ok "重命名分类" || bad "重命名分类" "$R"
[ -d "${NOTES}/阅读" ] && ok "目录已重命名" || bad "目录重命名" ""

echo "=== 18. 删除分类 ==="
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt -X DELETE "$BASE/api/categories/%E9%98%85%E8%AF%BB")
echo "$R" | grep -q '"ok":true' && ok "删除分类" || bad "删除分类" "$R"
[ ! -d "${NOTES}/阅读" ] && ok "目录已移除" || bad "目录移除" ""

echo "=== 18.5 Markdown 渲染与代码高亮 ==="
# payload 用独立的 python 脚本生成，避免 shell/python 双层引号转义问题。
# python 是原生进程，需要 Windows 路径，不能用 Git Bash 的 /c/... 形式。
WIN_WORK=$(cygpath -w "$WORK" 2>/dev/null || echo "$WORK")
cat > "$WORK"/mkpayload.py <<'PYEOF_PAYLOAD'
import json, sys, io
mode, out = sys.argv[1], sys.argv[2]
if mode == 'code':
    md = "```go\nfunc main() { fmt.Println(\"hi\") }\n```\n"
else:
    md = "inline `code` test"
io.open(out, 'w', encoding='utf-8').write(json.dumps({'markdown': md}))
PYEOF_PAYLOAD
python "$WORK"/mkpayload.py code "$WIN_WORK"/hl.json
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt -X POST $BASE/api/render \
  -H 'Content-Type: application/json' --data-binary @"$WORK"/hl.json)
HL=$(printf '%s' "$R" | grep -c 'cnchroma')
[ "$HL" -gt 0 ] && ok "代码块高亮输出 class" || bad "代码高亮" "无 class 输出"
printf '%s' "$R" | grep -q 'cnchroma' && ok "代码块保留 pre 包裹" || bad "pre 包裹" "缺失"
printf '%s' "$R" | grep -q 'cnkd' && ok "Go 关键字被识别" || bad "语言识别" "未见 cnkd"
# 行内代码不应被高亮破坏
python "$WORK"/mkpayload.py inline "$WIN_WORK"/inline.json
R=$(curl -s --noproxy "*" -b "$WORK"/cookie.txt -X POST $BASE/api/render \
  -H 'Content-Type: application/json' --data-binary @"$WORK"/inline.json)
printf '%s' "$R" | grep -q 'code' && ok "行内代码正常" || bad "行内代码" "未渲染"
# 高亮主题 CSS
CODE=$(curl -s --noproxy "*" -o /dev/null -w "%{http_code}" "$BASE/api/highlight.css?theme=github")
chk "高亮 CSS 接口" "$CODE" "200"
CSS=$(curl -s --noproxy "*" "$BASE/api/highlight.css?theme=github")
printf '%s' "$CSS" | grep -q '.cnchroma' && ok "高亮 CSS 含 chroma 选择器" || bad "CSS 内容" "缺少选择器"
CSS_DARK=$(curl -s --noproxy "*" "$BASE/api/highlight.css?theme=github-dark")
[ "$CSS" != "$CSS_DARK" ] && ok "亮暗主题不同" || bad "主题切换" "亮暗相同"

echo "=== 19. 静态页面 ==="
CODE=$(curl -s --noproxy "*" -o /dev/null -w "%{http_code}" $BASE/)
chk "首页可访问" "$CODE" "200"
R=$(curl -s --noproxy "*" $BASE/)
echo "$R" | grep -q 'CrazyNote' && ok "首页内容" || bad "首页内容" ""
CODE=$(curl -s --noproxy "*" -o /dev/null -w "%{http_code}" $BASE/assets/app.js)
chk "JS资源可访问" "$CODE" "200"
CODE=$(curl -s --noproxy "*" -o /dev/null -w "%{http_code}" $BASE/assets/app.css)
chk "CSS资源可访问" "$CODE" "200"
CODE=$(curl -s --noproxy "*" -o /dev/null -w "%{http_code}" $BASE/assets/highlight-light.css)
chk "高亮主题CSS可访问" "$CODE" "200"
CODE=$(curl -s --noproxy "*" -o /dev/null -w "%{http_code}" $BASE/assets/highlight-dark.css)
chk "高亮暗色CSS可访问" "$CODE" "200"

echo ""
echo "================================"
echo "通过: $PASS  失败: $FAIL"
echo "================================"
[ $FAIL -eq 0 ]
