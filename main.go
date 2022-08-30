package main

import (
	"fmt"
	"log"
	"os"
	. "superpose-sync/adapters"
	"superpose-sync/adapters/ConfigFile"
	"superpose-sync/adapters/inotify"
	"superpose-sync/adapters/sqlite"
	"superpose-sync/repositories"
	"superpose-sync/services/GoogleAPI"
	"superpose-sync/services/SaveGoogleInfo"

	"github.com/urfave/cli/v2" // https://cli.urfave.org/v2/
)

const MY_NAME = "Superpose"

const configDir = "."

const watchersFile = configDir + "/watchers.yml"

func main() {
	app := &cli.App{
		Name:  "superpose",
		Usage: "In quantum superposition a molecule can be in two (or more) quantun states before measurement. I do this with your files :D",
		Action: func(*cli.Context) error {
			fmt.Println("\033[32mboom! I say!\033[39m")
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}

	start()
}

func start() {
	err := ConfigFile.ParseFile(watchersFile)
	if err != nil {
		log.Fatal(err)
	}

	sqlite.Connect()

	Drive = GoogleAPI.NewDrive()
	Activity = GoogleAPI.NewActivity(Drive)

	Activity.ReceiveRemoteEvents("1ydbCRFUpgeoOi0ELU_vhZutROxwcdwMI", "edit")

	// startWatchers()
}

var (
	watcher *WatcherStruct
)

func startWatchers() {
	var err error

	watcher, err = Watcher()
	if err != nil {
		log.Fatal(err)
	}

	watcher.InotifyWatcher.StartWatch(receiveEvents)
	SaveGoogleInfo.StartListener()
	Activity.StartRemoteWatch()
}

type EventPath struct {
	Name    string
	Mask    uint32
	Watcher inotify.WatchingPath
	DriveID string
}

var (
	EventPaths = map[string]EventPath{}
	Drive      = GoogleAPI.GoogleDrive{}
	Activity   = GoogleAPI.GoogleDriveActivity{}
)

func (eventPath EventPath) Is(needle uint32) bool {
	return eventPath.Mask&needle == needle
}

func receiveEvents(event inotify.FileEvent) {
	var eventPath EventPath

	eventPath, ok := EventPaths[event.Name]
	if !ok {
		id, err := repositories.GetIdByPath(event.Name)
		if err != nil {
			log.Println("repositories.GetIdByPath error: ", err)
		}
		eventPath = EventPath{
			Name:    event.Name,
			DriveID: id,
		}
	}

	eventPath.Mask += event.Mask
	EventPaths[event.Name] = eventPath

	if event.Is(inotify.InCloseWrite) || event.Is(inotify.InDelete) || event.Is(inotify.InDeleteSelf) || event.Is(inotify.InMove) {
		delete(EventPaths, eventPath.Name)
		syncLocalToRemote(eventPath)
	}
}

func syncLocalToRemote(eventPath EventPath) {
	if eventPath.Is(inotify.InDelete) || eventPath.Is(inotify.InMovedFrom) {
		Drive.Remove(eventPath.DriveID)
		return
	}

	if eventPath.Is(inotify.InCloseWrite) {
		Drive.Send(eventPath.Name)
	}
}
