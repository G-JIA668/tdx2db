// Package report 生成 tdx2db 数据完整性检查的 HTML 报告。
// 纯 Go、无数据库依赖：输入为整理好的网格数据，输出单文件静态 HTML
// （内嵌数据 + JS/CSS，无 CDN 依赖，浏览器直接打开）。
package report

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// PartCount 分时网格中"部分完整"的交易日：当天分钟数 1~239。
type PartCount struct {
	I int `json:"i"` // 交易日索引
	C int `json:"c"` // 当天分钟条数
}

// Segment 一段连续缺失的交易日（首尾索引均含）。
type Segment struct {
	A int `json:"a"`
	B int `json:"b"`
	L int `json:"l"` // 缺失交易日个数
}

// SymbolRow 一个品种一行。
type SymbolRow struct {
	Symbol string      `json:"s"`
	Name   string      `json:"n"`
	First  int         `json:"f"` // 期望起点（首个数据日的索引）；-1 = 窗口内无数据
	Bits   string      `json:"b"` // base64 位图（1=有数据），[First:] 以外恒为 0
	Parts  []PartCount `json:"pc,omitempty"`
	Miss   []Segment   `json:"m"`
	Exp    int         `json:"e"`
	Pres   int         `json:"p"`
}

// DayMiss 市场级缺失日：某交易日有多少个品种缺数据。
type DayMiss struct {
	I int `json:"i"`
	N int `json:"n"`
}

// GridData 一个检查维度（日线 / 分时）的完整数据。
type GridData struct {
	Kind   string      `json:"kind"` // "daily" | "min"
	Days   []string    `json:"days"` // 期望交易日（"2006-01-02"）
	Rows   []SymbolRow `json:"rows"`
	Absent []SymbolRow `json:"absent"` // 窗口内无任何数据的品种（不进网格，仅清单）
	Mbd    []DayMiss   `json:"mbd"`    // 缺失日排行（按缺失品种数降序）
	Warn   []string    `json:"warn"`
}

// Report 一份完整报告。
type Report struct {
	GeneratedAt string    `json:"t"`
	Warnings    []string  `json:"w"`
	Daily       *GridData `json:"daily"`
	Min         *GridData `json:"min"`
}

// BuildDaily 由采集结果构建日线网格。
// universe 为全部品种代码；present[sym][date] 表示当日有日线 bar。
func BuildDaily(days []string, universe []string, present map[string]map[string]bool, names map[string]string) *GridData {
	syms := sortedCopy(universe)
	g := buildGrid("daily", days, syms, names, func(sym, date string) (ok bool, partial bool, count int) {
		return present[sym][date], false, 0
	})
	return g
}

// BuildMin 由采集结果构建分时网格。
// counts[sym][date] 为当日分钟条数；fullMin 为"完整交易日"的分钟阈值。
func BuildMin(days []string, universe []string, counts map[string]map[string]int64, names map[string]string, fullMin int) *GridData {
	syms := sortedCopy(universe)
	g := buildGrid("min", days, syms, names, func(sym, date string) (ok bool, partial bool, count int) {
		n := counts[sym][date]
		if n <= 0 {
			return false, false, 0
		}
		if int(n) < fullMin {
			return true, true, int(n)
		}
		return true, false, int(n)
	})
	return g
}

func sortedCopy(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

func buildGrid(kind string, days []string, universe []string, names map[string]string,
	state func(sym, date string) (ok bool, partial bool, count int)) *GridData {

	g := &GridData{Kind: kind, Days: days}
	// 以下字段始终非 nil：无缺失/无缺席品种时 JSON 输出 [] 而非 null（前端直接取 .length）。
	g.Rows = []SymbolRow{}
	g.Absent = []SymbolRow{}
	g.Mbd = []DayMiss{}
	mbd := make([]int, len(days))

	for _, sym := range universe {
		bits := make([]bool, len(days))
		first := -1
		pres := 0
		var parts []PartCount
		for i, d := range days {
			ok, partial, cnt := state(sym, d)
			if !ok {
				continue
			}
			bits[i] = true
			pres++
			if first < 0 {
				first = i
			}
			if partial {
				parts = append(parts, PartCount{I: i, C: cnt})
			}
		}

		row := SymbolRow{Symbol: sym, Name: names[sym]}
		if first < 0 {
			g.Absent = append(g.Absent, row)
			continue
		}
		row.First = first
		row.Bits = packBits(bits)
		row.Parts = parts
		row.Miss = missSegments(bits, first)
		row.Exp = len(days) - first
		row.Pres = pres
		for i := first; i < len(bits); i++ {
			if !bits[i] {
				mbd[i]++
			}
		}
		g.Rows = append(g.Rows, row)
	}

	// 缺失日排行（降序）
	idx := make([]int, 0, len(days))
	for i, n := range mbd {
		if n > 0 {
			idx = append(idx, i)
		}
	}
	sort.Slice(idx, func(a, b int) bool { return mbd[idx[a]] > mbd[idx[b]] })
	for _, i := range idx {
		g.Mbd = append(g.Mbd, DayMiss{I: i, N: mbd[i]})
	}

	label := "日线"
	if kind == "min" {
		label = "分时"
	}
	g.Warn = truncationWarning(days, g.Rows, label)
	return g
}

// truncationWarning 检测"历史截断"：绝大多数品种的首个数据日一致且明显晚于窗口起点，
// 说明更可能是历史未导入完整，而不是集中上市。
func truncationWarning(days []string, rows []SymbolRow, label string) []string {
	if len(rows) == 0 {
		return nil
	}
	firstCount := map[int]int{}
	for _, r := range rows {
		if r.First >= 0 {
			firstCount[r.First]++
		}
	}
	topIdx, topN := 0, 0
	for i, n := range firstCount {
		if n > topN {
			topIdx, topN = i, n
		}
	}
	if topN*10 >= len(rows)*8 && topIdx > 20 {
		return []string{strconv.Itoa(topN) + " 个品种（" + strconv.Itoa(100*topN/len(rows)) + "%）的" + label + "都从 " +
			days[topIdx] + " 才开始，疑似未导入完整历史"}
	}
	return nil
}

func packBits(bits []bool) string {
	n := (len(bits) + 7) / 8
	buf := make([]byte, n)
	for i, b := range bits {
		if b {
			buf[i>>3] |= 1 << (7 - (i & 7))
		}
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func missSegments(bits []bool, first int) []Segment {
	// 始终返回非 nil 切片：无缺失时 JSON 输出 [] 而非 null（前端直接取 .length）。
	out := []Segment{}
	inSeg := false
	start := 0
	for i := first; i < len(bits); i++ {
		if !bits[i] {
			if !inSeg {
				inSeg, start = true, i
			}
		} else if inSeg {
			out = append(out, Segment{A: start, B: i - 1, L: i - start})
			inSeg = false
		}
	}
	if inSeg {
		out = append(out, Segment{A: start, B: len(bits) - 1, L: len(bits) - start})
	}
	return out
}

// Render 生成单文件 HTML 报告。
func Render(rep *Report) (string, error) {
	b, err := json.Marshal(rep)
	if err != nil {
		return "", err
	}
	// 防 </script> 提前闭合
	js := strings.Replace(string(b), "<", "\\u003c", -1)
	return strings.Replace(htmlTemplate, "/*__REPORT_JSON__*/", js, 1), nil
}
