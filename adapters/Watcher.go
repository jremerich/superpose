package Watcher

import (
	"superpose-sync/adapters/ConfigFile"
	"superpose-sync/adapters/inotify"
	"superpose-sync/utils"
)

type WatcherStruct struct {
	InotifyWatcher *inotify.InotifyWatcher
}

func Watcher() (*WatcherStruct, error) {
	inotifyWatcher, err := inotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	watcher := &WatcherStruct{
		InotifyWatcher: inotifyWatcher,
	}

	err = watcher.CreateWatchers()
	if err != nil {
		return nil, err
	}

	return watcher, nil
}

func (watcher *WatcherStruct) CreateWatchers() error {
	for _, path := range ConfigFile.Configs.WatchPaths {
		strPath, err := utils.GetAbsPath(path.Path)
		if err != nil {
			return err
		}

		isIgnored, err := ConfigFile.PathInIgnore(strPath)
		if err != nil {
			return err
		}

		if !isIgnored {
			isRecursive := true
			if path.Recursive != nil {
				isRecursive = *path.Recursive
			}

			mask := inotify.InStringToMask(ConfigFile.Configs.Mask)
			if path.Mask != nil {
				mask = inotify.InStringToMask(*path.Mask)
			}

			err = watcher.InotifyWatcher.AddWatcher(strPath, isRecursive, mask)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
