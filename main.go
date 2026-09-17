// Knockbox —— 自建消息推送服务端。
//
// 它只负责消息：存储、推送、历史、富媒体。
// 谁该收哪条消息是上游系统的事，这里没有「订阅」这个概念。
package main

import "github.com/aichy126/knockbox/internal/cli"

// Version 由构建时 -ldflags "-X main.Version=..." 注入。
var Version = "dev"

func main() {
	cli.Version = Version
	cli.Execute()
}
