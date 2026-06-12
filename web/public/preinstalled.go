package public

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

// preinstalledThemes 存放随二进制内置发布的主题（构建产物），
// 例如 PurCarte。每个子目录是一个主题，结构与上传的主题包一致：
// preinstalledThemes/<short>/{komari-theme.json, preview.png, dist/...}
//
//go:embed preinstalledThemes
var preinstalledThemesFS embed.FS

// SyncPreinstalledThemes 在启动时把内置预置主题同步到 ./data/theme/<short>/。
//
// 仅覆盖主题文件，绝不触碰：
//   - 数据库中的主题个性化设置（ThemeConfiguration，按 short 存）
//   - 当前启用的主题（config: theme）
//
// 这样每次部署新二进制，内置主题文件会随之更新，而用户在面板里调好的
// 个性化配置和当前主题选择都保持不变。
func SyncPreinstalledThemes() {
	const root = "preinstalledThemes"

	themes, err := fs.ReadDir(preinstalledThemesFS, root)
	if err != nil {
		// 没有预置主题（目录为空或不存在），直接跳过。
		return
	}

	for _, themeDir := range themes {
		if !themeDir.IsDir() {
			continue
		}
		short := themeDir.Name()
		srcBase := root + "/" + short
		dstBase := filepath.Join(DataDir, ThemesDir, short)

		walkErr := fs.WalkDir(preinstalledThemesFS, srcBase, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(srcBase, p)
			if err != nil {
				return err
			}
			target := filepath.Join(dstBase, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0755)
			}
			data, err := preinstalledThemesFS.ReadFile(p)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			return os.WriteFile(target, data, 0644)
		})

		if walkErr != nil {
			log.Printf("同步预置主题 %s 失败: %v", short, walkErr)
			continue
		}
		log.Printf("已同步预置主题: %s -> %s", short, dstBase)
	}
}
