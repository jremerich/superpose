package SaveGoogleInfo

import (
	"log"
	"strings"
	"superpose-sync/repositories"
	"superpose-sync/services/GoogleAPI"

	drive "google.golang.org/api/drive/v3"
)

func StartListener() {
	go func() {
		for {
			select {
			case event, ok := <-GoogleAPI.ChannelDriveEvents:
				if !ok {
					log.Println("!ok: ", ok)
					return
				}

				path := repositories.Path{
					ID:        event.File.Id,
					Name:      event.File.Name,
					MimeType:  event.File.MimeType,
					ChangedAt: event.File.ModifiedTime,
					CreatedAt: event.File.CreatedTime,
					IsDir:     getIsDir(event.File),
					ParentID:  getParentId(event.File),
					FullPath:  event.File.AppProperties["fullPath"],
				}

				log.Printf("event.Action: %v", event.Action)
				// log.Println("path: ", path.String())

				if strings.HasSuffix(event.Action, ".Remove") {
					repositories.Delete(path)
				} else {
					repositories.Upsert(path)
				}
			}
		}
	}()
}

func getParentId(file drive.File) string {
	if len(file.Parents) > 0 {
		return file.Parents[0]
	}

	return ""
}

func getIsDir(file drive.File) int {
	if file.MimeType == "application/vnd.google-apps.folder" {
		return 1
	}
	return 0
}
