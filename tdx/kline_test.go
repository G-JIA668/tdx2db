package tdx

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDayVolume(t *testing.T) {
	tests := []struct {
		name     string
		volRaw   uint32
		reserved uint32
		want     int64
	}{
		{"modern reserved", 189_000_000, 0x10000, 189_000_000},
		{"early sz reserved zero", 25_859_600, 0x00000000, 25_859_600},
		{"early sh reserved 1000", 174_085_000, 0x000003E8, 174_085_000},
		{"early sh reserved 3139", 40_631_800, 0x00000C43, 40_631_800},
		{"nonzero low byte", 478_000_000, 0x0000002A, 478_000_000},
		{"tdx hands marker", 47_846_559, 0xc364002f, 4_784_655_947},
		{"tdx hands marker exact", 47_846_560, 0xc3640000, 4_784_656_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDayVolume(tt.volRaw, tt.reserved)
			if got != tt.want {
				t.Errorf("parseDayVolume(%d, 0x%x) = %d, want %d", tt.volRaw, tt.reserved, got, tt.want)
			}
		})
	}
}

func TestProcessDayFileIgnoresReservedForVolume(t *testing.T) {
	data := make([]byte, recordSize)
	binary.LittleEndian.PutUint32(data[0:4], 19991110)
	binary.LittleEndian.PutUint32(data[4:8], 2775)
	binary.LittleEndian.PutUint32(data[8:12], 2775)
	binary.LittleEndian.PutUint32(data[12:16], 2775)
	binary.LittleEndian.PutUint32(data[16:20], 2775)
	binary.LittleEndian.PutUint32(data[20:24], math.Float32bits(float32(4_859_102_208)))
	binary.LittleEndian.PutUint32(data[24:28], 174_085_000)
	binary.LittleEndian.PutUint32(data[28:32], 0x000003E8)

	rows, err := processDayFile(data, "sh600000")
	if err != nil {
		t.Fatalf("processDayFile: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Volume != 174_085_000 {
		t.Errorf("volume = %d, want 174085000", rows[0].Volume)
	}
	if rows[0].Close != 27.75 {
		t.Errorf("close = %f, want 27.75", rows[0].Close)
	}
	if rows[0].UpCount != 0 || rows[0].DownCount != 0 {
		t.Errorf("breadth = (%d, %d), want (0, 0)", rows[0].UpCount, rows[0].DownCount)
	}
}

func TestProcessDayFileScalesC364VolumeMarker(t *testing.T) {
	data := make([]byte, recordSize)
	binary.LittleEndian.PutUint32(data[0:4], 20260312)
	binary.LittleEndian.PutUint32(data[4:8], 352)
	binary.LittleEndian.PutUint32(data[8:12], 380)
	binary.LittleEndian.PutUint32(data[12:16], 350)
	binary.LittleEndian.PutUint32(data[16:20], 380)
	binary.LittleEndian.PutUint32(data[20:24], math.Float32bits(float32(17_458_274_304)))
	binary.LittleEndian.PutUint32(data[24:28], 47_846_559)
	binary.LittleEndian.PutUint32(data[28:32], 0xc364002f)

	rows, err := processDayFile(data, "sh601868")
	if err != nil {
		t.Fatalf("processDayFile: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Volume != 4_784_655_947 {
		t.Errorf("volume = %d, want 4784655947", rows[0].Volume)
	}
	if rows[0].Close != 3.8 {
		t.Errorf("close = %f, want 3.8", rows[0].Close)
	}
}

func TestProcessDayFileParsesIndexBreadth(t *testing.T) {
	data := make([]byte, recordSize)
	binary.LittleEndian.PutUint32(data[0:4], 20260618)
	binary.LittleEndian.PutUint32(data[4:8], 292649)
	binary.LittleEndian.PutUint32(data[8:12], 293535)
	binary.LittleEndian.PutUint32(data[12:16], 291939)
	binary.LittleEndian.PutUint32(data[16:20], 292875)
	binary.LittleEndian.PutUint32(data[24:28], 123_456)
	binary.LittleEndian.PutUint32(data[28:32], 0x0026000c)

	rows, err := processDayFile(data, "sh000016")
	if err != nil {
		t.Fatalf("processDayFile: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Volume != 123_456 {
		t.Errorf("volume = %d, want 123456", rows[0].Volume)
	}
	if rows[0].UpCount != 12 || rows[0].DownCount != 38 {
		t.Errorf("breadth = (%d, %d), want (12, 38)", rows[0].UpCount, rows[0].DownCount)
	}
}

func TestProcessDayFileParsesBlockBreadth(t *testing.T) {
	data := make([]byte, recordSize)
	binary.LittleEndian.PutUint32(data[0:4], 20260618)
	binary.LittleEndian.PutUint32(data[24:28], 123_456)
	binary.LittleEndian.PutUint32(data[28:32], 0x003b0024)

	rows, err := processDayFile(data, "sh881044")
	if err != nil {
		t.Fatalf("processDayFile: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].UpCount != 36 || rows[0].DownCount != 59 {
		t.Errorf("breadth = (%d, %d), want (36, 59)", rows[0].UpCount, rows[0].DownCount)
	}
}

func TestProcessMinFileInt(t *testing.T) {
	// .01（datatool 输出）：OHLC 为整数价格，需除以 PriceScale。
	data := make([]byte, recordSize)
	binary.LittleEndian.PutUint16(data[0:2], (2026-2004)*2048+6*100+24) // 20260624
	binary.LittleEndian.PutUint16(data[2:4], 9*60+31)                   // 09:31
	binary.LittleEndian.PutUint32(data[4:8], 2775)
	binary.LittleEndian.PutUint32(data[8:12], 2800)
	binary.LittleEndian.PutUint32(data[12:16], 2760)
	binary.LittleEndian.PutUint32(data[16:20], 2790)
	binary.LittleEndian.PutUint32(data[20:24], math.Float32bits(float32(12345678)))
	binary.LittleEndian.PutUint32(data[24:28], 100000)

	rows, err := processMinFileInt(data, "sh600000")
	if err != nil {
		t.Fatalf("processMinFile: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Close != 27.9 {
		t.Errorf("close = %f, want 27.9", row.Close)
	}
	if row.Volume != 100000 {
		t.Errorf("volume = %d, want 100000", row.Volume)
	}
	if row.Amount != 12345678 {
		t.Errorf("amount = %f, want 12345678", row.Amount)
	}
	want := time.Date(2026, 6, 24, 9, 31, 0, 0, time.Local)
	if !row.Datetime.Equal(want) {
		t.Errorf("datetime = %v, want %v", row.Datetime, want)
	}
}

func TestMinPriceEncodingDifference(t *testing.T) {
	// 同一段字节（float32 位型 9.0 = 0x41100000）在两种编码下的解读：
	// .lc1（客户端 minline）应为 9.0；.01（datatool 整数价格）应为 0x41100000/100。
	// 混用曾导致真实 .lc1 价格被放大 ~1.2e6 倍（2026-09-24 事件）。
	data := make([]byte, recordSize)
	binary.LittleEndian.PutUint16(data[0:2], (2026-2004)*2048+9*100+24) // 20260924
	binary.LittleEndian.PutUint16(data[2:4], 14*60+58)                  // 14:58
	bits := math.Float32bits(float32(9.0))
	binary.LittleEndian.PutUint32(data[4:8], bits)
	binary.LittleEndian.PutUint32(data[8:12], bits)
	binary.LittleEndian.PutUint32(data[12:16], bits)
	binary.LittleEndian.PutUint32(data[16:20], bits)
	binary.LittleEndian.PutUint32(data[24:28], 6900)

	rows, err := processMinFileFloat(data, "sh600000")
	if err != nil {
		t.Fatalf("processMinFileFloat: %v", err)
	}
	if len(rows) != 1 || rows[0].Close != 9.0 {
		t.Fatalf("float parse rows = %+v, want close 9.0", rows)
	}

	rows, err = processMinFileInt(data, "sh600000")
	if err != nil {
		t.Fatalf("processMinFileInt: %v", err)
	}
	if len(rows) != 1 || rows[0].Close != float64(bits)/100 {
		t.Fatalf("int parse rows = %+v, want close %f", rows, float64(bits)/100)
	}
}

func TestCollectAndConvertFileList(t *testing.T) {
	data := make([]byte, recordSize)
	binary.LittleEndian.PutUint16(data[0:2], (2026-2004)*2048+6*100+24) // 20260624
	binary.LittleEndian.PutUint16(data[2:4], 9*60+31)                   // 09:31
	binary.LittleEndian.PutUint32(data[4:8], math.Float32bits(float32(27.75)))
	binary.LittleEndian.PutUint32(data[8:12], math.Float32bits(float32(27.75)))
	binary.LittleEndian.PutUint32(data[12:16], math.Float32bits(float32(27.75)))
	binary.LittleEndian.PutUint32(data[16:20], math.Float32bits(float32(27.75)))
	binary.LittleEndian.PutUint32(data[24:28], 100000)

	dir := t.TempDir()
	sub := filepath.Join(dir, "sh", "minline")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"sh600001.lc1", "sh600000.lc1", "sh600002.lc5"} {
		if err := os.WriteFile(filepath.Join(sub, f), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := CollectKlineFiles(dir, ".lc1")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2 (.lc5 不应被收集)", len(files))
	}
	if !strings.HasSuffix(files[0], "sh600000.lc1") || !strings.HasSuffix(files[1], "sh600001.lc1") {
		t.Fatalf("files not sorted: %v", files)
	}

	csvPath := filepath.Join(dir, "out.csv")
	if _, err := ConvertFileListToCSV(context.Background(), files, csvPath, ".lc1"); err != nil {
		t.Fatalf("ConvertFileListToCSV: %v", err)
	}
	b, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Fatalf("csv lines = %d, want 3:\n%s", len(lines), b)
	}
}

func TestConvertFilesToCSVSupportsLc1(t *testing.T) {
	data := make([]byte, recordSize)
	binary.LittleEndian.PutUint16(data[0:2], (2026-2004)*2048+6*100+24) // 20260624
	binary.LittleEndian.PutUint16(data[2:4], 9*60+31)                   // 09:31
	binary.LittleEndian.PutUint32(data[4:8], math.Float32bits(float32(27.75)))
	binary.LittleEndian.PutUint32(data[8:12], math.Float32bits(float32(27.75)))
	binary.LittleEndian.PutUint32(data[12:16], math.Float32bits(float32(27.75)))
	binary.LittleEndian.PutUint32(data[16:20], math.Float32bits(float32(27.75)))
	binary.LittleEndian.PutUint32(data[24:28], 100000)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sh600000.lc1"), data, 0644); err != nil {
		t.Fatal(err)
	}
	// 干扰文件：后缀不同，不应被收集
	if err := os.WriteFile(filepath.Join(dir, "sh600001.lc5"), data, 0644); err != nil {
		t.Fatal(err)
	}

	csvPath := filepath.Join(dir, "out.csv")
	if _, err := ConvertFilesToCSV(context.Background(), dir, csvPath, ".lc1"); err != nil {
		t.Fatalf("ConvertFilesToCSV .lc1: %v", err)
	}

	b, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("csv lines = %d, want 2 (header + 1 row):\n%s", len(lines), b)
	}
	if !strings.HasPrefix(lines[0], "symbol,open,high,low,close,amount,volume,datetime") {
		t.Errorf("header = %q", lines[0])
	}
	fields := strings.Split(lines[1], ",")
	if fields[0] != "sh600000" || fields[4] != "27.75" {
		t.Errorf("row = %q", lines[1])
	}
	// datetime 按 RFC3339 写出，解析回来应与输入一致
	dt, err := time.Parse(time.RFC3339, fields[7])
	if err != nil {
		t.Fatalf("parse datetime %q: %v", fields[7], err)
	}
	want := time.Date(2026, 6, 24, 9, 31, 0, 0, time.Local)
	if !dt.Equal(want) {
		t.Errorf("datetime = %v, want %v", dt, want)
	}
}
