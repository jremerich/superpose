package GoogleAPI

import (
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"superpose-sync/adapters/ConfigFile"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/option"
)

type GoogleDriveActivity struct {
	service      driveactivity.Service
	driveService GoogleDrive
}

type driveItemAux struct {
	Name     string
	Title    string
	MimeType string
}

var (
	receivedEventsIds         map[string]string = map[string]string{}
	googleDriveActivity       GoogleDriveActivity
	atomicGoogleDriveActivity uint64
	mutexGoogleDriveActivity  = &sync.Mutex{}
)

func (driveItem driveItemAux) getId() string {
	if strings.HasPrefix(driveItem.Name, "items/") {
		return strings.Replace(driveItem.Name, "items/", "", 1)
	}

	return driveItem.Name
}

func NewGoogleDriveActivityService(Drive GoogleDrive) GoogleDriveActivity {
	if atomic.LoadUint64(&atomicGoogleDriveActivity) == 1 {
		return googleDriveActivity
	}

	mutexGoogleDriveActivity.Lock()
	defer mutexGoogleDriveActivity.Unlock()

	if atomicGoogleDriveActivity == 0 {
		service, err := driveactivity.NewService(getContext(), option.WithHTTPClient(getOAuthClient()))
		if err != nil {
			log.Fatalf("Unable to create Drive service: %v", err)
		}

		googleDriveActivity = GoogleDriveActivity{
			service:      *service,
			driveService: Drive,
		}

		atomic.StoreUint64(&atomicGoogleDriveActivity, 1)
	}
	return googleDriveActivity
}

func (googleDriveActivity *GoogleDriveActivity) StartRemoteWatch() {
	wait := make(chan struct{})
	for {
		go googleDriveActivity.callQueryActivities(wait)
		<-wait
	}
}

func (googleDriveActivity *GoogleDriveActivity) callQueryActivities(ch chan struct{}) {
	for k := range receivedEventsIds {
		delete(receivedEventsIds, k)
	}

	q := driveactivity.QueryDriveActivityRequest{
		AncestorName: fmt.Sprintf("items/%s", ConfigFile.Configs.GoogleDrive.RootFolderId),
		Filter:       fmt.Sprintf("time >= \"%s\"", ConfigFile.Configs.GoogleDrive.LastActivityCheck),
	}

	r, err := googleDriveActivity.service.Activity.Query(&q).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve list of activities. %v", err)
	}
	if len(r.Activities) > 0 {
		for _, a := range r.Activities {
			for i := range a.Targets {
				if a.Targets[i].DriveItem != nil {
					driveItem := getDriveItem(a.Targets[i].DriveItem)

					driveItemID := driveItem.getId()
					action := getOneOf(*a.Actions[i].Detail)

					_, ok := receivedEventsIds[driveItemID]
					if !ok {
						receivedEventsIds[driveItemID] = action
						googleDriveActivity.ReceiveRemoteEvents(driveItemID, action)
					}
				}
			}
		}
	}

	ConfigFile.SetLastActivityCheck()

	time.Sleep(5 * time.Second)
	ch <- struct{}{}
}

func getDriveItem(target *driveactivity.DriveItem) driveItemAux {
	return driveItemAux{
		Name:     target.Name,
		Title:    target.Title,
		MimeType: target.MimeType,
	}
}

func (googleDriveActivity *GoogleDriveActivity) ReceiveRemoteEvents(driveItemID, action string) {
	driveFile, err := googleDriveActivity.driveService.GetFile(driveItemID)
	if err != nil {
		log.Printf("Got Files.List error: %#v, %v", driveFile, err)
	} else {
		sendEventMessage(DriveEvent{
			File:   *driveFile,
			Action: action,
		})
	}
}

func (googleDriveActivity *GoogleDriveActivity) DownloadToLocal(driveItemID, fileName, fileMode string) {
	driveFile, err := googleDriveActivity.driveService.DownloadFile(driveItemID)
	if err != nil {
		log.Printf("Got Drive.DownloadFile error: %#v, %v", driveFile, err)
	} else {
		defer driveFile.Body.Close()
		out, err := os.Create(fileName)
		if err != nil {
			log.Printf("Got os.Create error: %#v, %v", driveFile, err)
		}
		defer out.Close()

		fmt.Printf("\nDownload File: %v\n", fileName)
		_, err = io.Copy(out, driveFile.Body)
		if err != nil {
			log.Printf("Got io.Copy error: %v", err)
		}

		if mode, err := strconv.ParseUint(fileMode, 0, 32); err == nil {
			fmt.Printf("%T, %v (%v)\n", mode, mode, fileMode)
			if err := os.Chmod(fileName, os.FileMode(mode)); err != nil {
				log.Fatal(err)
			}
		}
	}
}

// Returns the name of a set property in an object, or else "unknown".
func getOneOf(m interface{}) string {
	v := reflect.ValueOf(m)
	for i := 0; i < v.NumField(); i++ {
		if !v.Field(i).IsNil() {
			return v.Type().Field(i).Name
		}
	}
	return "unknown"
}
