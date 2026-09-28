package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jing2uo/tdx2db/database"
	"github.com/jing2uo/tdx2db/model"
	"github.com/jing2uo/tdx2db/report"
	"github.com/jing2uo/tdx2db/workflow"
)

// minFullBars 是一个完整交易日的分钟 bar 数（09:30-11:30 + 13:00-15:00）。
const minFullBars = 240

// Check 检查数据库数据完整性（近 years 年日线 / 近 minMonths 个月分时），
// 生成自包含 HTML 报告。只读诊断：不改写被检查的库。
func Check(ctx context.Context, dbURI, outPath string, years, minMonths int) error {
	db, err := database.NewDB(dbURI)
	if err != nil {
		return fmt.Errorf("failed to create database driver: %w", err)
	}
	if err := db.Connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	if err := ctx.Err(); err != nil {
		return err
	}

	var warnings []string
	ver, verErr := db.ReadSchemaVersion()
	switch {
	case verErr != nil:
		warnings = append(warnings, "无法读取 schema 版本（"+verErr.Error()+"），库可能不完整，检查继续")
	case ver == "":
		warnings = append(warnings, "库缺少 schema 版本记录（_meta），检查继续")
	default:
		verMajor, err := strconv.Atoi(strings.SplitN(ver, ".", 2)[0])
		if err != nil {
			return fmt.Errorf("invalid schema version format: %q", ver)
		}
		if verMajor != model.SchemaMajor {
			return schemaVersionIncompatible(verMajor, model.SchemaMajor)
		}
	}

	today := GetToday()

	// 交易日历：raw_holidays 为空/不可读时回退纯工作日（法定假日会被误标为缺失）。
	holidays, herr := db.GetHolidays()
	if herr != nil {
		warnings = append(warnings, "读取 raw_holidays 失败（"+herr.Error()+"），按纯工作日日历检查")
		holidays = nil
	} else if len(holidays) == 0 {
		warnings = append(warnings, "raw_holidays 为空，按纯工作日日历检查，法定假日会被误标为缺失")
	}
	cal := workflow.NewTradingCalendar(holidays)

	// 检查窗口：起点 = today - N 年/月；终点 = 今天之前（含今天）的最近交易日。
	// 今天尚未导入会整列标红，直观提示补跑 cron；导入后即正常计绿。
	end := cal.LastTradingDayOnOrBefore(today)
	dailyDays := tradingDays(cal, today.AddDate(-years, 0, 0), end)
	minDays := tradingDays(cal, today.AddDate(0, -minMonths, 0), end)
	fmt.Printf("📅 日线窗口 %s ~ %s（%d 个交易日）\n", dailyDays[0], dailyDays[len(dailyDays)-1], len(dailyDays))
	fmt.Printf("📅 分时窗口 %s ~ %s（%d 个交易日）\n", minDays[0], minDays[len(minDays)-1], len(minDays))

	// 采集
	dailyUniverse, dailyPresent, err := collectDaily(db, dailyDays[0])
	if err != nil {
		return err
	}
	minUniverse, minCounts, err := collectMin(db, minDays[0])
	if err != nil {
		return err
	}
	names := collectNames(db)
	fmt.Printf("📦 日线 %d 个品种 / 分时 %d 个品种\n", len(dailyUniverse), len(minUniverse))

	daily := report.BuildDaily(dailyDays, dailyUniverse, dailyPresent, names)
	min := report.BuildMin(minDays, minUniverse, minCounts, names, minFullBars)

	rep := &report.Report{
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
		Warnings:    warnings,
		Daily:       daily,
		Min:         min,
	}
	html, err := report.Render(rep)
	if err != nil {
		return fmt.Errorf("failed to render report: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	fmt.Printf("📊 报告已生成: %s\n", outPath)
	fmt.Printf("   日线: %d 个品种，%d 个有缺口，覆盖度 %s\n", len(daily.Rows), countWithMiss(daily), coverageOf(daily))
	fmt.Printf("   分时: %d 个品种，%d 个有缺口，覆盖度 %s\n", len(min.Rows), countWithMiss(min), coverageOf(min))
	return nil
}

// tradingDays 枚举 [start, end] 内所有交易日，返回 "2006-01-02" 列表。
func tradingDays(cal *workflow.TradingCalendar, start, end time.Time) []string {
	var days []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if cal.IsTradingDay(d) {
			days = append(days, d.Format("2006-01-02"))
		}
	}
	return days
}

type symRow struct {
	Symbol string `db:"symbol"`
}

type klineDateRow struct {
	Symbol string    `db:"symbol"`
	Date   time.Time `db:"date"`
}

type minCountRow struct {
	Symbol string    `db:"symbol"`
	D      time.Time `db:"d"`
	N      int64     `db:"n"`
}

type nameRow struct {
	Symbol string `db:"symbol"`
	Name   string `db:"name"`
}

func collectDaily(db database.DataRepository, since string) ([]string, map[string]map[string]bool, error) {
	var universe []symRow
	if err := db.QueryRaw("SELECT DISTINCT symbol FROM "+model.TableKlineDaily.TableName, &universe); err != nil {
		return nil, nil, fmt.Errorf("query daily universe: %w", err)
	}
	var rows []klineDateRow
	if err := db.QueryRaw("SELECT symbol, date FROM "+model.TableKlineDaily.TableName+" WHERE date >= ?", &rows, since); err != nil {
		return nil, nil, fmt.Errorf("query daily presence: %w", err)
	}
	present := make(map[string]map[string]bool, len(universe))
	for _, r := range rows {
		m, ok := present[r.Symbol]
		if !ok {
			m = make(map[string]bool)
			present[r.Symbol] = m
		}
		m[r.Date.Format("2006-01-02")] = true
	}
	syms := make([]string, len(universe))
	for i, u := range universe {
		syms[i] = u.Symbol
	}
	return syms, present, nil
}

func collectMin(db database.DataRepository, since string) ([]string, map[string]map[string]int64, error) {
	var universe []symRow
	if err := db.QueryRaw("SELECT DISTINCT symbol FROM "+model.TableKline1Min.TableName, &universe); err != nil {
		return nil, nil, fmt.Errorf("query 1min universe: %w", err)
	}
	var rows []minCountRow
	query := "SELECT symbol, CAST(datetime AS DATE) AS d, count(*) AS n FROM " +
		model.TableKline1Min.TableName + " WHERE datetime >= ? GROUP BY symbol, d"
	if err := db.QueryRaw(query, &rows, since); err != nil {
		return nil, nil, fmt.Errorf("query 1min presence: %w", err)
	}
	counts := make(map[string]map[string]int64, len(universe))
	for _, r := range rows {
		m, ok := counts[r.Symbol]
		if !ok {
			m = make(map[string]int64)
			counts[r.Symbol] = m
		}
		m[r.D.Format("2006-01-02")] = r.N
	}
	syms := make([]string, len(universe))
	for i, u := range universe {
		syms[i] = u.Symbol
	}
	return syms, counts, nil
}

func collectNames(db database.DataRepository) map[string]string {
	var rows []nameRow
	if err := db.QueryRaw("SELECT symbol, name FROM "+model.TableSymbolName.TableName, &rows); err != nil {
		fmt.Printf("⚠️  读取代码名称失败（%v），报告仅显示代码\n", err)
		return nil
	}
	names := make(map[string]string, len(rows))
	for _, r := range rows {
		names[r.Symbol] = r.Name
	}
	return names
}

func countWithMiss(g *report.GridData) int {
	n := 0
	for _, r := range g.Rows {
		if len(r.Miss) > 0 {
			n++
		}
	}
	return n
}

func coverageOf(g *report.GridData) string {
	var pres, exp int
	for _, r := range g.Rows {
		pres += r.Pres
		exp += r.Exp
	}
	if exp == 0 {
		return "0.00%"
	}
	return fmt.Sprintf("%.2f%%", float64(pres)/float64(exp)*100)
}
