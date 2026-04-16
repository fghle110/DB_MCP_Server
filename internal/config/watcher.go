package config

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// StartWatcher 启动配置文件监听
func StartWatcher(app *AppState, onChange func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}

	configPath := app.ConfigPath()
	configFile := filepath.Base(configPath)
	configDir := filepath.Dir(configPath)

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// 检查事件是否针对配置文件（匹配文件名）
				eventBase := filepath.Base(event.Name)
				if eventBase != configFile {
					continue
				}
				// 处理 Write、Create、Rename 事件（覆盖写/重建文件场景）
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
					log.Printf("[config] detected config change, reloading...")
					if err := ReloadConfig(app); err != nil {
						log.Printf("[config] reload failed: %v", err)
					} else if onChange != nil {
						onChange()
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("[config] watcher error: %v", err)
			}
		}
	}()

	// 监听配置文件所在目录，而不是文件本身（跨平台更可靠）
	if err := watcher.Add(configDir); err != nil {
		return fmt.Errorf("watch config dir %s: %w", configDir, err)
	}

	log.Printf("[config] watching %s for changes", configPath)
	return nil
}

// ReloadConfig 热重载配置
func ReloadConfig(app *AppState) error {
	// 先备份
	if err := BackupConfig(app.ConfigPath()); err != nil {
		log.Printf("[config] backup failed: %v", err)
	}

	// 加载新配置
	newCfg, err := LoadConfig(app.ConfigPath())
	if err != nil {
		app.UpdateReloadFailed()
		return fmt.Errorf("load config: %w", err)
	}

	// 原子替换
	app.UpdateConfig(newCfg)
	log.Printf("[config] config reloaded successfully")
	return nil
}
