// protogen 从 packages/protocol/protocol.json 单源生成双端协议登记副本：
//
//	go run ./cmd/protogen -root <项目根>
//
// 产出（幂等）：client/scripts/net/protocol.gd、server/internal/protocol/protocol.go。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	root := flag.String("root", ".", "项目根目录")
	flag.Parse()
	if err := run(*root); err != nil {
		fmt.Fprintln(os.Stderr, "protogen:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	reg, err := LoadRegistry(filepath.Join(root, "packages", "protocol", "protocol.json"))
	if err != nil {
		return err
	}
	gd, err := GenerateGDScript(reg)
	if err != nil {
		return err
	}
	goSrc, err := GenerateGo(reg)
	if err != nil {
		return err
	}
	outDir := filepath.Join(root, "server", "internal", "protocol")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	targets := []struct {
		path string
		body string
	}{
		{filepath.Join(root, "client", "scripts", "net", "protocol.gd"), gd},
		{filepath.Join(outDir, "protocol.go"), goSrc},
	}
	for _, t := range targets {
		if err := os.WriteFile(t.path, []byte(t.body), 0o644); err != nil {
			return fmt.Errorf("写 %s: %w", t.path, err)
		}
		fmt.Println("written", t.path)
	}
	return nil
}
