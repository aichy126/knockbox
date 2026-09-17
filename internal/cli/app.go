package cli

import (
	"fmt"

	"github.com/aichy126/igo"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/migrate"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

// openApp 初始化 igo 并把 schema 迁到最新。
// 每个子命令都走它，所以 CLI 操作永远不会碰到过期的 schema。
func openApp() (*igo.Application, *xorm.Engine, error) {
	// 显式传路径：留空的话 igo 会去解析 stdlib 的 -c flag，和 cobra 打架。
	app, err := igo.NewApp(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("加载配置 %s 失败: %w", configPath, err)
	}
	dm := app.DB.Get(models.DBName)
	if dm == nil || dm.WriteDB == nil {
		return nil, nil, fmt.Errorf("配置里找不到 [sqlite.%s]，参考 config.toml.example", models.DBName)
	}
	if err := migrate.Run(dm.WriteDB); err != nil {
		return nil, nil, fmt.Errorf("数据库迁移失败: %w", err)
	}
	return app, dm.WriteDB, nil
}

func openDAO() (*igo.Application, *dao.DAO, error) {
	app, engine, err := openApp()
	if err != nil {
		return nil, nil, err
	}
	return app, dao.New(engine), nil
}
