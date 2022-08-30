package GoogleAPI

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"superpose-sync/adapters/ConfigFile"
	"superpose-sync/repositories"
	"superpose-sync/utils"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	drive "google.golang.org/api/drive/v3"
)

type DrivePrepared struct {
	fileCreateCall *drive.FilesCreateCall
}

type GoogleDrive struct {
	service drive.Service
}

type DriveEvent struct {
	File   drive.File
	Action string
}

var (
	ErrNotFound        = errors.New("drive: path doesn't exists")
	ChannelDriveEvents chan DriveEvent
	filesFields        googleapi.Field = "id, name, mimeType, parents, createdTime, modifiedTime, appProperties, properties"
)

func init() {
	log.Println("Initiating GoogleDrive API Service")
	ChannelDriveEvents = make(chan DriveEvent)
}

func NewGoogleDriveService() GoogleDrive {
	service, err := drive.NewService(getContext(), option.WithHTTPClient(getOAuthClient()))
	if err != nil {
		log.Fatalf("Unable to create Drive service: %v", err)
	}

	googleDrive := GoogleDrive{
		service: *service,
	}
	return googleDrive
}

func (googleDrive *GoogleDrive) Remove(id string) error {
	log.Println("Remove: ", id)
	file := &drive.File{
		Id: id,
	}
	log.Println("drive.File criado")
	_, err := Do(file, googleDrive.service.Files.Delete(id).Do())
	// err := googleDrive.service.Files.Delete(id).Do()
	log.Println("removido: ", err)
	return err
}

func (googleDrive *GoogleDrive) Send(filename string) error {
	info, err := os.Stat(filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	goFile, err := os.Open(filename)
	if err != nil {
		log.Printf("error opening %q: %v", filename, err)
		return err
	}

	parentId := googleDrive.CreateTree(filename, info)

	fileId, driveFile, err := googleDrive.FileExists(filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if fileId != "" {
		driveFile, err = Do(googleDrive.Update(fileId, driveFile).Media(goFile).Fields(filesFields).Do())
		if err != nil {
			log.Printf("Got drive.File, err: %#v, %v", driveFile, err)
			return err
		}
		return nil
	}

	log.Printf("\u001B[32m[%s] filename: %s | parentId: %s | FileInfo.Name(): %s | FileInfo.Mode(): %s | FileInfo.Mode().Perm(): %s\u001B[39m\n",
		utils.GetFunctionName(), filename, parentId, info.Name(), info.Mode(), info.Mode().Perm())

	driveFile, err = googleDrive.CreateFile(info.Name(), parentId, filename).Media(goFile).Do()
	if err != nil {
		log.Printf("Got drive.File, err: %#v, %v", driveFile, err)
		return err
	}

	return nil
}

func (p DrivePrepared) Do(opts ...googleapi.CallOption) (*drive.File, error) {
	file, err := Do(p.fileCreateCall.Fields(filesFields).Do(opts...))
	return file, err
}

func Do(file *drive.File, err error) (*drive.File, error) {
	if err != nil {
		log.Printf("Got drive.File, err: %#v, %v", file, err)
	} else {
		sendEventMessage(DriveEvent{
			File:   *file,
			Action: utils.GetFunctionNameSkip(1),
		})
	}
	return file, err
}

func (p DrivePrepared) Media(r io.Reader, options ...googleapi.MediaOption) DrivePrepared {
	p.fileCreateCall = p.fileCreateCall.Media(r, options...)
	return p
}

func (googleDrive *GoogleDrive) FileExists(fullPath string) (string, *drive.File, error) {
	var file *drive.File
	parentId, err := repositories.GetIdByPath(fullPath)
	if errors.Is(err, sql.ErrNoRows) {
		parentId, file, err = googleDrive.GetIdByPath(fullPath, "")
		if err != nil {
			return "", nil, err
		}
	}
	return parentId, file, nil
}

func (googleDrive *GoogleDrive) CreateTree(fullPath string, info os.FileInfo) string {
	log.Printf("[%s] fullPath: %v\n", utils.GetFunctionName(), fullPath)
	parentId, _, err := googleDrive.FileExists(fullPath)
	if err == nil {
		return parentId
	}

	if info == nil {
		_info, err := os.Stat(fullPath)
		if err != nil {
			log.Fatalln(err)
		}
		info = _info
	}
	path := fullPath
	if !info.IsDir() {
		path = filepath.Dir(fullPath)
	}
	path = utils.GetAbsPathRemote(path)

	pathList := strings.Split(path, "/")

	parentId = ""
	actualPathTree := []string{}
	for _, dir := range pathList {
		actualPathTree = append(actualPathTree, dir)
		if dir == utils.REMOTE_PREFIX {
			parentId = ConfigFile.Configs.GoogleDrive.RootFolderId
			continue
		}

		strActualPathTree := filepath.Join(actualPathTree...)
		_parentId, err := repositories.GetIdByPath(strActualPathTree)
		if errors.Is(err, sql.ErrNoRows) {
			_parentId, _, err = googleDrive.GetIdByPath(strActualPathTree, parentId)
			if errors.Is(err, ErrNotFound) {
				folder, errCreate := googleDrive.CreateFile(dir, parentId, strActualPathTree).Do()
				if errCreate != nil {
					log.Fatalln(errCreate)
				}
				_parentId = folder.Id
			}
		}
		parentId = _parentId
	}

	return parentId
}

func (googleDrive *GoogleDrive) Update(fileId string, file *drive.File) *drive.FilesUpdateCall {
	return googleDrive.service.Files.Update(fileId, file)
}

func (googleDrive *GoogleDrive) CreateFile(name string, parentId string, path string) DrivePrepared {
	file := generateDriveFile(name, parentId, path)
	return DrivePrepared{fileCreateCall: googleDrive.service.Files.Create(file)}
}

func generateDriveFile(name string, parentId string, path string) *drive.File {
	path = utils.GetAbsPathLocal(path)
	info, err := os.Stat(path)
	if err != nil {
		log.Fatalf("Unable to Stat path: %#v, %v", path, err)
	}
	appProperties := generateAppProperties(path, info)

	driveFile := &drive.File{
		Name:             name,
		AppProperties:    appProperties,
		Properties:       appProperties,
		ModifiedTime:     info.ModTime().Format(time.RFC3339),
		OriginalFilename: path,
		Parents:          []string{parentId},
	}

	if info.IsDir() {
		driveFile.MimeType = "application/vnd.google-apps.folder"
	}

	driveFile = applyDescription(driveFile)

	return driveFile
}

func applyDescription(f *drive.File) *drive.File {
	props := f.AppProperties
	strDescription := ""
	for key, value := range props {
		strDescription += key + ": " + value + "\n"
	}
	f.Description = strDescription
	return f
}

func generateAppProperties(path string, info os.FileInfo) map[string]string {
	path = utils.GetAbsPathLocal(path)

	appProperties := map[string]string{
		"fullPath":  path,
		"mode":      fmt.Sprintf("%04o", info.Mode().Perm()),
		"changedAt": info.ModTime().String(),
	}

	return appProperties
}

func (googleDrive *GoogleDrive) ListAll() (*drive.FileList, error) {
	return googleDrive.GetList("")
}

func (googleDrive *GoogleDrive) GetFile(fileId string) (*drive.File, error) {
	file, err := googleDrive.service.Files.Get(fileId).Fields(filesFields).Do()

	if err != nil {
		log.Printf("Got Files.List error: %#v, %v", file, err)
		return nil, err
	}
	return file, nil
}

func (googleDrive *GoogleDrive) DownloadFile(fileId string) (*http.Response, error) {
	file, err := googleDrive.service.Files.Get(fileId).Download()

	if err != nil {
		log.Printf("Got Files.List error: %#v, %v", file, err)
		return nil, err
	}
	return file, nil
}

func (googleDrive *GoogleDrive) GetList(query string) (*drive.FileList, error) {
	filesListCall := googleDrive.service.Files.List()
	if query != "" {
		filesListCall = filesListCall.Q(query)
	}

	var fields = "files(" + filesFields + ")"
	fileList, err := filesListCall.Fields(fields).Do()
	if err != nil {
		log.Printf("Got Files.List error: %#v, %v", fileList, err)
		return nil, err
	}

	if len(fileList.Files) == 0 {
		return nil, ErrNotFound
	}

	return fileList, nil
}

func (googleDrive *GoogleDrive) GetIdByPath(fullPath string, parentId string) (string, *drive.File, error) {
	fullPath = utils.GetAbsPathLocal(fullPath)
	query := "appProperties has { key='fullPath' and value='" + fullPath + "' }"
	if parentId != "" {
		query += " and '" + parentId + "' in parents"
	}
	fileList, err := googleDrive.GetList(query)
	if err != nil {
		log.Println("GetIdByPath err: ", err)
		return "", nil, err
	}

	if len(fileList.Files) == 0 {
		return "", nil, ErrNotFound
	}

	sendEventMessage(DriveEvent{
		File:   *fileList.Files[0],
		Action: utils.GetFunctionName(),
	})

	return fileList.Files[0].Id, fileList.Files[0], nil
}

func sendEventMessage(event DriveEvent) {
	ChannelDriveEvents <- event
}

var Changes []*drive.Change

// func (googleDrive *GoogleDrive) StartRemoteWatch(callback func(event DriveEvent)) {
// func (googleDrive *GoogleDrive) StartRemoteWatch() {
// 	pageToken := "1657316" // googleDrive.getPageToken()
// 	googleDrive.nextChangesPage(pageToken)

// 	for _, change := range Changes {
// 		file := change.File
// 		jsonContent, _ := json.Marshal(file)
// 		log.Printf("File: %v", string(jsonContent))
// 	}
// }

// nextPageToken,newStartPageToken,changes(removed, file(id, name, mimeType, parents, createdTime, modifiedTime, appProperties, properties))
// nextPageToken,newStartPageToken,changes(fileId,file(name,parents,mimeType))

// func (googleDrive *GoogleDrive) getPageToken() string {
// 	if ConfigFile.Configs.GoogleDrive.StartPageToken != "" {
// 		log.Println("Saved startPageToken: ", ConfigFile.Configs.GoogleDrive.StartPageToken)
// 		return ConfigFile.Configs.GoogleDrive.StartPageToken
// 	}
// 	startPageToken, err := googleDrive.service.Changes.GetStartPageToken().Do()
// 	if err != nil {
// 		log.Println("GetStartPageToken error: ", err)
// 	}
// 	log.Println("startPageToken: ", startPageToken.StartPageToken)
// 	return startPageToken.StartPageToken
// }

// func (googleDrive *GoogleDrive) nextChangesPage(pageToken string) {
// 	var fields = "nextPageToken,newStartPageToken,changes(removed, file(" + filesFields + "))"
// 	list, err := googleDrive.service.Changes.List(pageToken).Fields(fields).Do()
// 	if err != nil {
// 		log.Println("List error: ", err)
// 	}

// 	Changes = append(Changes, list.Changes...)

// 	if list.NextPageToken != "" {
// 		googleDrive.nextChangesPage(list.NextPageToken)
// 	} else {
// 		log.Println("NewStartPageToken: ", list.NewStartPageToken)
// 		ConfigFile.Configs.GoogleDrive.StartPageToken = list.NewStartPageToken
// 		ConfigFile.SaveFile()
// 	}
// }
