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

func (driveItem driveItemAux) getId() string {
	if strings.HasPrefix(driveItem.Name, "items/") {
		return strings.Replace(driveItem.Name, "items/", "", 1)
	}

	return driveItem.Name
}

func NewGoogleDriveActivityService(Drive GoogleDrive) GoogleDriveActivity {
	service, err := driveactivity.NewService(getContext(), option.WithHTTPClient(getOAuthClient()))
	if err != nil {
		log.Fatalf("Unable to create Drive service: %v", err)
	}

	googleDriveActivity := GoogleDriveActivity{
		service:      *service,
		driveService: Drive,
	}
	return googleDriveActivity
}

var receivedEventsIds map[string]string = map[string]string{}

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
	ConfigFile.Configs.GoogleDrive.LastActivityCheck = time.Now().UTC().Format(time.RFC3339)
	ConfigFile.SaveFile()

	r, err := googleDriveActivity.service.Activity.Query(&q).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve list of activities. %v", err)
	}
	if len(r.Activities) > 0 {
		for _, a := range r.Activities {
			for i := range a.Targets {
				if a.Targets[i].DriveItem != nil {
					var driveItem driveItemAux

					if a.Targets[i].DriveItem.DriveFile != nil {
						driveFile := a.Targets[i].DriveItem
						driveItem = driveItemAux{
							Name:     driveFile.Name,
							Title:    driveFile.Title,
							MimeType: driveFile.MimeType,
						}
					} else if a.Targets[i].DriveItem.DriveFolder != nil {
						driveFolder := a.Targets[i].DriveItem
						driveItem = driveItemAux{
							Name:     driveFolder.Name,
							Title:    driveFolder.Title,
							MimeType: driveFolder.MimeType,
						}
					}
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

	time.Sleep(5 * time.Second)
	ch <- struct{}{}
}

func (googleDriveActivity *GoogleDriveActivity) ReceiveRemoteEvents(driveItemID, action string) {
	driveFile, err := googleDriveActivity.driveService.GetFile(driveItemID)
	if err != nil {
		log.Printf("Got Files.List error: %#v, %v", driveFile, err)
	} else {
		fileName := driveFile.AppProperties["fullPath"]
		fileMode := driveFile.AppProperties["mode"]
		if action == "Delete" {
			deleteLocalFile(fileName)
		} else {
			googleDriveActivity.downloadToLocal(driveItemID, fileName, fileMode)
		}
	}
}

func (googleDriveActivity *GoogleDriveActivity) downloadToLocal(driveItemID, fileName, fileMode string) {
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

func deleteLocalFile(fileName string) {
	// remove do diretorio local
	log.Printf("\nDelete File: %v", fileName)
	err := os.Remove(fileName)
	if err != nil {
		log.Printf("Got os.Remove error: %#v, %v", fileName, err)
	}
}

// Returns the type of a target and an associated title.
func getTargetInfo(target *driveactivity.Target) string {
	if target.DriveItem != nil {
		return fmt.Sprintf("driveItem:\"%s\"", target.DriveItem.Name)
	}
	if target.Drive != nil {
		return fmt.Sprintf("drive:\"%s\"", target.Drive.Name)
	}
	if target.FileComment != nil {
		parent := target.FileComment.Parent
		if parent != nil {
			return fmt.Sprintf("fileComment:\"%s\"", parent.Name)
		}
		return "fileComment:unknown"
	}
	return getOneOf(*target)
}

// Returns information for a list of targets.
func getTargetsInfo(targets []*driveactivity.Target) []string {
	targetsInfo := make([]string, len(targets))
	for i := range targets {
		targetsInfo[i] = getTargetInfo(targets[i])
	}
	return targetsInfo
}

// Returns a string representation of the first elements in a list.
func truncated(array []string) string {
	return truncatedTo(array, 2)
}

// Returns a string representation of the first elements in a list.
func truncatedTo(array []string, limit int) string {
	var contents string
	var more string
	if len(array) <= limit {
		contents = strings.Join(array, ", ")
		more = ""
	} else {
		contents = strings.Join(array[0:limit], ", ")
		more = ", ..."
	}
	return fmt.Sprintf("[%s%s]", contents, more)
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

// Returns a time associated with an activity.
func getTimeInfo(activity *driveactivity.DriveActivity) string {
	if activity.Timestamp != "" {
		return activity.Timestamp
	}
	if activity.TimeRange != nil {
		return activity.TimeRange.EndTime
	}
	return "unknown"
}

// Returns the type of action.
func getActionInfo(action *driveactivity.ActionDetail) string {
	return getOneOf(*action)
}

// Returns user information, or the type of user if not a known user.
func getUserInfo(user *driveactivity.User) string {
	if user.KnownUser != nil {
		if user.KnownUser.IsCurrentUser {
			return "people/me"
		}
		return user.KnownUser.PersonName
	}
	return getOneOf(*user)
}

// Returns actor information, or the type of actor if not a user.
func getActorInfo(actor *driveactivity.Actor) string {
	if actor.User != nil {
		return getUserInfo(actor.User)
	}
	return getOneOf(*actor)
}

// Returns information for a list of actors.
func getActorsInfo(actors []*driveactivity.Actor) []string {
	actorsInfo := make([]string, len(actors))
	for i := range actors {
		actorsInfo[i] = getActorInfo(actors[i])
	}
	return actorsInfo
}
