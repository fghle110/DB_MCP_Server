package config

import (
	"fmt"
	"log"

	"github.com/fsnotify/fsnotify"
)

// StartWatcher 启动配置文件监听
func StartWatcher(app *AppState, onChange func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					if event.Name == app.ConfigPath() || event.Name == app.ConfigPath()+".tmp" {
						log.Printf("[config] detected config change, reloading...")
						if err := ReloadConfig(app); err != nil {
							log.Printf("[config] reload failed: %v", err)
						} else if onChange != nil {
							onChange()
						}
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

	if err := watcher.Add(app.ConfigPath()); err != nil {
		return fmt.Errorf("watch config file: %w", err)
	}

	log.Printf("[config] watching %s for changes", app.ConfigPath())
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
