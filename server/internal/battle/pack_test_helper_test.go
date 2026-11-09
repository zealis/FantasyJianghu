package battle

import (
	"path/filepath"
	"testing"

	"fantasy-jianghu/server/internal/config"
)

// packViaTool 用仓库真实源打包一份配置包并返回目录（属性测试同源校验）。
func packViaTool(t *testing.T) string {
	t.Helper()
	out := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Pack(filepath.Join(root, "configs", "source"), out, "9.9.0"); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	return filepath.Join(out, "9.9.0")
}
