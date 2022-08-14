package utils

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

func GetAbsPath(path string) (string, error) {
	pathName := path
	homeDir, err := os.UserHomeDir()

	if err != nil {
		return "", err
	}

	if pathName == "~" {
		// In case of "~", which won't be caught by the "else if"
		pathName = homeDir
	} else if strings.HasPrefix(pathName, "~/") {
		// Use strings.HasPrefix so we don't match pathNames like
		// "/something/~/something/"
		pathName = filepath.Join(homeDir, pathName[2:])
	}

	return pathName, nil
}

func GetAbsPathRemote(path string) string {
	if !strings.HasPrefix(path, REMOTE_PREFIX) {
		path = REMOTE_PREFIX + path
	}
	return path
}

func GetAbsPathLocal(path string) string {
	if strings.HasPrefix(path, REMOTE_PREFIX) {
		path = strings.ReplaceAll(path, REMOTE_PREFIX, "")
	}
	var err error
	path, err = GetAbsPath(path)
	if err != nil {
		log.Fatalf("Unable to GetAbsPathLocal: %v", err)
	}
	return path
}
