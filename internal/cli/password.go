package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// readPassword 按三种方式取密码，优先级从高到低：
//
//	--password        显式的非交互用法（CI / 脚本）。会进 shell history 和 ps，所以不是默认。
//	管道 / 重定向      echo 'pw' | knockbox user add alice
//	交互式隐藏输入      默认。要求输入两次，避免把打错的密码设进去还不知道。
//
// 三种都支持，是为了不必为了自动化而被迫把密码写进命令行参数。
func readPassword(flagValue string, confirm bool) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	// 不是终端就当管道读。这样 CI 里可以安全地喂密码，不经过命令行参数。
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("从标准输入读密码失败（要交互式输入请在终端里运行）")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	pw, err := promptHidden("密码: ")
	if err != nil {
		return "", err
	}
	if confirm {
		again, err := promptHidden("再输一次: ")
		if err != nil {
			return "", err
		}
		if pw != again {
			return "", errors.New("两次输入不一致")
		}
	}
	return pw, nil
}

func promptHidden(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("读取密码失败: %w", err)
	}
	return string(b), nil
}
