package GoogleAPI

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"superpose-sync/adapters/ConfigFile"

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

func (googleDriveActivity *GoogleDriveActivity) StartRemoteWatch(callback func(driveItemID, action string)) {
	// func (googleDriveActivity *GoogleDriveActivity) StartRemoteWatch() {
	q := driveactivity.QueryDriveActivityRequest{
		// PageSize:     10,
		AncestorName: fmt.Sprintf("items/%s", ConfigFile.Configs.GoogleDrive.RootFolderId),
		Filter:       fmt.Sprintf("time >= \"%s\"", "2022-08-17T04:00:53.375Z"),
	}
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
						// driveItem = a.Targets[i].DriveItem.DriveFolder
						driveFolder := a.Targets[i].DriveItem
						driveItem = driveItemAux{
							Name:     driveFolder.Name,
							Title:    driveFolder.Title,
							MimeType: driveFolder.MimeType,
						}
					}
					driveItemID := driveItem.getId()
					action := getOneOf(*a.Actions[i].Detail)
					callback(driveItemID, action)
				}
			}
		}
	} else {
		fmt.Print("No activity.")
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

func ReceiveRemoteEvents(event DriveEvent) {

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
