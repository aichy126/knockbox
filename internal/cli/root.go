// Package cli 命令行入口。
//
// 自建服务的运维只能靠命令行，所以这里守四条：
//
//  1. 任何账号操作都有对应的子命令。重设管理员密码这类基本操作，不应该要求
//     使用者自己生成 bcrypt 哈希再去 UPDATE 数据库。
//  2. 密码默认交互式隐藏输入，同时支持管道。写在命令行参数里会短暂出现在
//     ps 输出中，因此不作为默认方式。
//  3. 能配置的项都有 CLI 入口，不留只能改库才能完成的操作。
//  4. flag 名扁平、不带点号，与环境变量的映射保持直观。
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version 由构建时 -ldflags 注入。
	Version = "dev"

	configPath string
)

func Execute() {
	root := &cobra.Command{
		Use:   "knockbox",
		Short: "自建消息推送服务端",
		Long: "Knockbox —— 自建消息推送服务端。\n\n" +
			"它只负责消息：存储、推送、历史、富媒体。\n" +
			"谁该收哪条消息是上游系统的事，这里没有订阅的概念。",
		SilenceUsage:  true, // 运行期错误不要糊一屏 usage 出来
		SilenceErrors: true, // 错误统一在下面打，保证格式一致
		Version:       Version,
	}
	root.PersistentFlags().StringVarP(&configPath, "config", "c", "config.toml", "配置文件路径")

	root.AddCommand(
		newServeCmd(),
		newUserCmd(),
		newPairCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
