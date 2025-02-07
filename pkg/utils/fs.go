package utils

import (
	"os"
	"path/filepath"
	"strings"
)

type fs struct{}

var FS = fs{}

func (f fs) GetNameWithoutExt(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func (f fs) PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}
