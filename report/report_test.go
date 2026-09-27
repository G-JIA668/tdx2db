package report

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// days 生成 n 个从 2026-01-05 起的连续日期（仅测试用，真实交易日历由 cmd 层生成）。
func days(n int) []string {
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.Local)
	out := make([]string, n)
	for i := range out {
		out[i] = start.AddDate(0, 0, i).Format("2006-01-02")
	}
	return out
}

func TestPackBitsRoundtrip(t *testing.T) {
	bits := make([]bool, 23)
	for _, i := range []int{0, 2, 7, 8, 22} {
		bits[i] = true
	}
	s := packBits(bits)
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i, want := range bits {
		got := (raw[i>>3] >> (7 - (i & 7))) & 1
		if int(got) != 0 != want {
			t.Errorf("bit %d = %d, want %v", i, got, want)
		}
	}
}

func TestBuildDaily(t *testing.T) {
	ds := days(10)
	present := map[string]map[string]bool{
		"sh000001": {ds[0]: true, ds[1]: true, ds[2]: true, ds[3]: true, ds[4]: true, ds[5]: true, ds[6]: true, ds[7]: true, ds[8]: true, ds[9]: true},
		"sh000002": {ds[0]: true, ds[1]: true, ds[2]: true, ds[3]: true, ds[7]: true, ds[8]: true, ds[9]: true}, // 缺 4~6
		// sh000003: universe 里有但窗口内无数据
	}
	g := BuildDaily(ds, []string{"sh000003", "sh000001", "sh000002"}, present, map[string]string{"sh000001": "平安"})

	if len(g.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(g.Rows))
	}
	if len(g.Absent) != 1 || g.Absent[0].Symbol != "sh000003" {
		t.Fatalf("absent = %+v, want sh000003", g.Absent)
	}
	// 排序后 sh000001 在前
	r1 := g.Rows[0]
	if r1.Symbol != "sh000001" || r1.Name != "平安" {
		t.Fatalf("row0 = %+v", r1)
	}
	if r1.Exp != 10 || r1.Pres != 10 || len(r1.Miss) != 0 {
		t.Fatalf("row0 exp=%d pres=%d miss=%v, want 10/10/0", r1.Exp, r1.Pres, r1.Miss)
	}
	r2 := g.Rows[1]
	if r2.Symbol != "sh000002" || r2.Pres != 7 || len(r2.Miss) != 1 {
		t.Fatalf("row1 = %+v, want pres=7 miss=1", r2)
	}
	seg := r2.Miss[0]
	if seg.A != 4 || seg.B != 6 || seg.L != 3 {
		t.Fatalf("segment = %+v, want 4~6 len 3", seg)
	}
	// 缺失日排行：4/5/6 各缺 1 个品种
	if len(g.Mbd) != 3 || g.Mbd[0].I != 4 || g.Mbd[0].N != 1 {
		t.Fatalf("mbd = %+v", g.Mbd)
	}
}

func TestBuildMinPartialAndFull(t *testing.T) {
	ds := days(3)
	counts := map[string]map[string]int64{
		"sh000001": {ds[0]: 240, ds[1]: 30, ds[2]: 0}, // 完整 / 部分 / 缺失
	}
	g := BuildMin(ds, []string{"sh000001"}, counts, nil, 240)
	if len(g.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(g.Rows))
	}
	r := g.Rows[0]
	if r.First != 0 || r.Exp != 3 || r.Pres != 2 {
		t.Fatalf("row = %+v, want exp=3 pres=2", r)
	}
	if len(r.Parts) != 1 || r.Parts[0].I != 1 || r.Parts[0].C != 30 {
		t.Fatalf("parts = %+v, want [{1 30}]", r.Parts)
	}
	if len(r.Miss) != 1 || r.Miss[0].A != 2 || r.Miss[0].B != 2 || r.Miss[0].L != 1 {
		t.Fatalf("miss = %+v, want day2 missing", r.Miss)
	}
}

func TestTruncationWarning(t *testing.T) {
	ds := days(100)
	present := map[string]map[string]bool{
		"sh000001": {ds[60]: true, ds[61]: true},
		"sh000002": {ds[60]: true},
		"sh000003": {ds[60]: true},
	}
	g := BuildDaily(ds, []string{"sh000001", "sh000002", "sh000003"}, present, nil)
	if len(g.Warn) != 1 {
		t.Fatalf("warn = %v, want truncation warning", g.Warn)
	}
	if !strings.Contains(g.Warn[0], "疑似未导入完整历史") {
		t.Fatalf("warn text = %q", g.Warn[0])
	}
}

func TestEmptyGridFieldsNonNull(t *testing.T) {
	// 无缺席品种 / 无缺失日时，Absent 与 Mbd 也必须序列化为 [] 而非 null（前端直接取 .length）。
	ds := days(5)
	g := BuildDaily(ds, []string{"sh000001"}, map[string]map[string]bool{"sh000001": {ds[0]: true, ds[1]: true, ds[2]: true, ds[3]: true, ds[4]: true}}, nil)
	if g.Absent == nil || len(g.Absent) != 0 {
		t.Fatalf("absent = %#v, want non-nil empty", g.Absent)
	}
	if g.Mbd == nil || len(g.Mbd) != 0 {
		t.Fatalf("mbd = %#v, want non-nil empty", g.Mbd)
	}
	if g.Rows == nil {
		t.Fatal("rows should be non-nil")
	}
	// JSON 中必须是 [] 而不是 null
	html, err := Render(&Report{GeneratedAt: "t", Warnings: nil, Daily: g, Min: BuildMin(ds, nil, nil, nil, 240)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `"absent":[]`) {
		t.Error("json absent should be [] not null")
	}
	if !strings.Contains(html, `"mbd":[]`) {
		t.Error("json mbd should be [] not null")
	}
}

func TestRenderSmoke(t *testing.T) {
	ds := days(5)
	g := BuildDaily(ds, []string{"sh600000"}, map[string]map[string]bool{"sh600000": {ds[0]: true}}, nil)
	min := BuildMin(ds, nil, nil, nil, 240)
	rep := &Report{
		GeneratedAt: "2026-09-27 10:00:00",
		Warnings:    []string{"raw_holidays 为空"},
		Daily:       g,
		Min:         min,
	}
	html, err := Render(rep)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"tdx2db 数据完整性报告", `"s":"sh600000"`, "raw_holidays 为空", "</html>"} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q", want)
		}
	}
	if strings.Contains(html, "__REPORT_JSON__") {
		t.Error("json marker not replaced")
	}
	if strings.Contains(html, "</script>") == false {
		t.Error("html missing closing script tag")
	}
}
