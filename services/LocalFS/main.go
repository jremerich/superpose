package LocalFS

import (
	"log"
	"os"
	"superpose-sync/services/GoogleAPI"
)

func StartListener() {
	log.Println("Start LocalFSHandler Listener")
	go func() {
		for {
			select {
			case event, ok := <-GoogleAPI.ChannelDriveEvents:
				if !ok {
					log.Println("!ok: ", ok)
					return
				}

				log.Println("LocalFSHandler Listener")

				LocalFSHandler(event)
			}
		}
	}()
}

func LocalFSHandler(driveEvent GoogleAPI.DriveEvent) {
	log.Println("LocalFSHandler")
	driveFile := driveEvent.File
	action := driveEvent.Action

	fileName := driveFile.AppProperties["fullPath"]
	fileMode := driveFile.AppProperties["mode"]
	if action == "Delete" {
		deleteLocalFile(fileName)
	} else {
		googleDrive := GoogleAPI.NewGoogleDriveService()
		googleDriveActivity := GoogleAPI.NewGoogleDriveActivityService(googleDrive)
		googleDriveActivity.DownloadToLocal(driveFile.Id, fileName, fileMode)
	}
}

func deleteLocalFile(fileName string) {
	// remove do diretorio local
	log.Printf("\nDelete File: %v", fileName)
	err := os.Remove(fileName)
	if err != nil {
		log.Printf("Got os.Remove error: %#v, %v", fileName, err)
	}
}
