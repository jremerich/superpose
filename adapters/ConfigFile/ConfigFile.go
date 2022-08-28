package ConfigFile

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"superpose-sync/utils"
	"time"

	"gopkg.in/yaml.v3"
)

type WatchPath struct {
	Path      string  `yaml:"dir"`
	Recursive *bool   `yaml:"recursive"`
	Mask      *string `yaml:"mask"`
}

type GoogleDrive struct {
	RootFolderId   string `yaml:"root_folder_id"`
	StartPageToken string `yaml:"start_page_token"`
	ClientId       string `yaml:"client_id"`
	ClientSecret   string `yaml:"client_secret"`
	Token          Token  `yaml:"token"`
}

type Token struct {
	AccessToken  string    `yaml:"access_token"`
	TokenType    string    `yaml:"token_type"`
	RefreshToken string    `yaml:"refresh_token"`
	Expiry       time.Time `yaml:"expiry"`
	ExpiresIn    int       `yaml:"expires_in"`
	Scope        string    `yaml:"scope"`
}

type ConfigsStruct struct {
	GoogleDrive GoogleDrive `yaml:"google_drive"`
	Mask        string      `yaml:"mask"`
	ConfigPath  string      `yaml:"config_path"`
	DbPath      string      `yaml:"db"`
	WatchPaths  []WatchPath `yaml:"watchers,flow"`
	IgnorePaths []WatchPath `yaml:"ignore,flow"`
}

var (
	Configs ConfigsStruct
	Info    os.FileInfo
)

func isDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) && strings.HasSuffix(path, "*") {
			return false, nil
		}
		//if errors.Is(err, )
		return false, err
	}

	return fileInfo.IsDir(), err
}

func ParseFile(configFile string) error {
	info, err := os.Stat(configFile)
	if err != nil {
		return err
	}

	Info = info

	yamlFile, err := ioutil.ReadFile(Info.Name())
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(yamlFile, &Configs)
	if err != nil {
		return err
	}

	Configs.DbPath = strings.Replace(Configs.DbPath, "$CONFIG_PATH", Configs.ConfigPath, -1)

	return nil
}

func SaveFile() error {
	Configs.DbPath = strings.Replace(Configs.DbPath, Configs.ConfigPath, "$CONFIG_PATH", -1)
	yamlContent, err := yaml.Marshal(&Configs)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(Info.Name(), yamlContent, Info.Mode())
	if err != nil {
		return err
	}

	return nil
}

func PathInIgnore(pathToCheck string) (bool, error) {
	for _, path := range Configs.IgnorePaths {
		strPath, err := utils.GetAbsPath(path.Path)
		if err != nil {
			return true, err
		}

		needToAddWildcard, err := isDirectory(strPath)
		if err != nil {
			return true, err
		}

		if needToAddWildcard {
			strPath += "/**/*"
		}

		isIgnored, err := filepath.Match(strPath, pathToCheck)
		if err != nil {
			return true, err
		}

		if isIgnored {
			return true, nil
		}
	}
	return false, nil
}
