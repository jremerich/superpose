package inotify

import (
	"log"
	"os"
	"path/filepath"
	Watcher "superpose-sync/adapters/ConfigFile"
)

// InotifyWatcher recursively watches the given root folder, waiting for file events.
// Events can be masked by providing fileMask. DirWatcher does not generate events for
// folders or subfolders.
type InotifyWatcher struct {
	i     *Inotify
	stopC chan struct{}
	C     chan FileEvent
}

func NewWatcher() (*InotifyWatcher, error) {
	i, err := NewInotify()
	if err != nil {
		return nil, err
	}

	watcher := &InotifyWatcher{
		i:     i,
		stopC: make(chan struct{}),
		C:     make(chan FileEvent),
	}

	return watcher, nil
}

func (watcher *InotifyWatcher) AddRecursive(dir string, fileMask uint32) error {
	return watcher.AddWatcher(dir, true, fileMask)
}

func (watcher *InotifyWatcher) Add(dir string, fileMask uint32) error {
	return watcher.AddWatcher(dir, false, fileMask)
}

type WatchingPath struct {
	Name      string
	Mask      uint32
	Recursive bool
	FileInfo  os.FileInfo
}

var (
	WatchingPaths = map[string]WatchingPath{}
)

func (watcher *InotifyWatcher) GetWatcher(pathName string) WatchingPath {
	return WatchingPaths[pathName]
}

func (watcher *InotifyWatcher) RmWatcher(pathName string) error {
	i := watcher.i
	err := i.RmWatch(pathName)
	if err != nil {
		return err
	}

	delete(WatchingPaths, pathName)
	return nil
}

func (watcher *InotifyWatcher) InotifyAddWatcher(path string, mask uint32) error {
	i := watcher.i

	isIgnored, err := Watcher.PathInIgnore(path)
	if err != nil {
		return err
	}

	if !isIgnored {
		return i.AddWatch(path, mask)
	}

	return nil
}

func (watcher *InotifyWatcher) AddWatcher(dir string, recursive bool, fileMask uint32) error {
	var err error

	i := watcher.i

	err = filepath.Walk(dir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		setWatchingPaths(path, f, recursive, fileMask)

		if !recursive && f.IsDir() && path != dir {
			return filepath.SkipDir
		}

		if f.IsDir() {
			//log.Printf("walking '%s' | %s", dir, path)
			return watcher.InotifyAddWatcher(path, InAllEvents)
		} else {
			return nil
		}
	})

	if err != nil {
		i.Close()
		return err
	}

	return nil
}

func setWatchingPaths(path string, f os.FileInfo, recursive bool, fileMask uint32) {
	WatchingPaths[path] = WatchingPath{
		Name:      path,
		Mask:      fileMask,
		Recursive: recursive,
		FileInfo:  f,
	}
}

func (watcher *InotifyWatcher) StartWatch(callback func(event FileEvent)) {
	done := make(chan bool)

	i := watcher.i
	events := make(chan FileEvent)

	go func() {
		for {
			raw, err := i.Read()
			if err != nil {
				log.Println("Erro read: ", err)
				close(events)
				return
			}

			for _, event := range raw {
				// Skip ignored events queued from removed watchers.yml
				if event.Mask&InIgnored == InIgnored {
					continue
				}

				// Add watch for folders created in watched folders (recursion)
				if WatchingPaths[event.Name].Recursive && event.Mask&(InCreate|InIsDir) == InCreate|InIsDir {
					err = watcher.AddWatcher(event.Name, WatchingPaths[event.Name].Recursive, WatchingPaths[event.Name].Mask)
					if err != nil {
						close(events)
						return
					}
				}

				if event.Mask&InCreate == InCreate && event.Mask&InIsDir != InIsDir {
					parentEvent := WatchingPaths[filepath.Dir(event.Name)]
					//log.Println("\nparentEvent.Mask: ", InMaskToString(parentEvent.Mask))
					fileInfo, err := os.Stat(event.Name)
					if err != nil {
						log.Println("Stat error for new file: ", err)
						close(events)
						return
					}
					setWatchingPaths(event.Name, fileInfo, parentEvent.Recursive, parentEvent.Mask)
				}

				// Remove watch for deleted folders
				if event.Mask&InDeleteSelf == InDeleteSelf {
					//err = i.RmWd(event.Wd)
					err = watcher.RmWatcher(filepath.Dir(event.Name))
					if err != nil {
						log.Println("RmWd 2 error: ", err)
						close(events)
						return
					}
				}

				// Skip sub-folder events
				if event.Mask&InIsDir == InIsDir && event.Mask&InCreate != InCreate {
					continue
				}

				events <- FileEvent{
					InotifyEvent: event,
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-watcher.stopC:
				log.Println("<-watcher.stopC 1")
				i.Close()
			case event, ok := <-events:
				if !ok {
					log.Println("Eof: true")
					watcher.C <- FileEvent{
						Eof: true,
					}
					return
				}

				// Skip events not conforming with provided mask
				fileMask := WatchingPaths[event.Name].Mask
				if event.Mask&fileMask == 0 {
					continue
				}

				watcher.C <- event
			}
		}
	}()

	go func() {
		for {
			select {
			case <-watcher.stopC:
				log.Println("<-watcher.stopC 2")
				i.Close()
			case event, ok := <-watcher.C:
				if !ok {
					log.Println("!ok: ", ok)
					return
				}

				callback(event)
			}
		}
	}()

	<-done
}

func (watcher *InotifyWatcher) Close() {
	select {
	case watcher.stopC <- struct{}{}:
	default:
	}
}
