package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/jing2uo/tdx2db/database"
	"github.com/jing2uo/tdx2db/model"
	"github.com/jing2uo/tdx2db/tdx"
	"github.com/jing2uo/tdx2db/utils"
)

// ImportMin 把本地 1 分钟 K 线文件（.01 / .lc1）导入 raw_kline_1min。
// 供历史分时补录使用：纯 Go 解析，不依赖 datatool，任何平台可用。
// raw_kline_1min 已有数据时需 --force，否则拒绝（导入是纯 INSERT，重复执行会翻倍）。
func ImportMin(ctx context.Context, dbURI, minFileDir string, force bool) error {
	db, err := database.NewDB(dbURI)
	if err != nil {
		return fmt.Errorf("failed to create database driver: %w", err)
	}

	if err := db.Connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	if err := db.InitSchema(); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	if err := writeSchemaVersion(db); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	fmt.Printf("📦 开始处理分时目录: %s\n", minFileDir)
	if err := utils.CheckDirectory(minFileDir); err != nil {
		return err
	}

	latest, err := db.GetLatestDate(model.TableKline1Min.TableName, "datetime")
	if err != nil {
		return fmt.Errorf("query 1min latest: %w", err)
	}
	if !latest.IsZero() && !force {
		return fmt.Errorf("raw_kline_1min 已有数据（最新 %s），重复导入会产生重复行；确认追加请加 --force",
			latest.Format("2006-01-02 15:04"))
	}

	found := false
	for _, suffix := range []string{".01", ".lc1"} {
		ok, err := importMinSuffix(ctx, db, minFileDir, suffix)
		if err != nil {
			return err
		}
		found = found || ok
	}
	if !found {
		return fmt.Errorf("目录下没有找到 .01 / .lc1 分时文件（文件名需形如 sh600000.lc1）")
	}

	fmt.Println("📊 分时数据导入成功")
	return nil
}

// importMinSuffix 转换并导入指定后缀的分时文件，返回目录下是否存在该后缀文件。
func importMinSuffix(ctx context.Context, db database.DataRepository, minFileDir, suffix string) (bool, error) {
	fmt.Printf("🐌 开始转换分时数据 (%s)\n", suffix)
	csvPath := filepath.Join(TempDir, "1min-"+strings.TrimPrefix(suffix, ".")+".csv")
	_, err := tdx.ConvertFilesToCSV(ctx, minFileDir, csvPath, suffix)
	if err != nil {
		return false, fmt.Errorf("failed to convert %s files to csv: %w", suffix, err)
	}

	// ConvertFilesToCSV 在目录下无匹配文件时不创建 CSV（提前返回），跳过导入。
	if _, err := os.Stat(csvPath); err != nil {
		fmt.Printf("🌲 目录下无 %s 文件，跳过\n", suffix)
		return false, nil
	}

	if err := db.ImportKline1Min(csvPath); err != nil {
		return false, fmt.Errorf("failed to import 1min csv: %w", err)
	}
	return true, nil
}
