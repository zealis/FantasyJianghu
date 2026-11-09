// configtool 为配置打包 CLI（开发技术规范 §7）：校验不通过不生成配置包。
//
//	go run ./cmd/configtool -source ./configs/source -out ./configs/build -version 1.0.0
package main

import (
	"flag"
	"fmt"
	"os"

	"fantasy-jianghu/server/internal/config"
)

func main() {
	source := flag.String("source", "./configs/source", "配置源目录")
	out := flag.String("out", "./configs/build", "配置包输出目录")
	version := flag.String("version", "", "配置包版本号")
	flag.Parse()
	if *version == "" {
		fmt.Fprintln(os.Stderr, "configtool: 需要 -version")
		os.Exit(2)
	}
	if err := config.Pack(*source, *out, *version); err != nil {
		fmt.Fprintln(os.Stderr, "configtool:", err)
		os.Exit(1)
	}
	fmt.Println("packed", "out:", *out, "version:", *version)
}
