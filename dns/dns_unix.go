// +build !windows

package dns

import (
	log "github.com/sirupsen/logrus"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
)

func DNS(ip string) error {
	var err error
	if err = os.MkdirAll(filepath.Join("/", "etc", "resolver"), fs.ModePerm); err != nil {
		log.Error(err)
	}
	filename := filepath.Join("/", "etc", "resolver", "local")
	fileContent := "nameserver " + ip
	return ioutil.WriteFile(filename, []byte(fileContent), fs.ModePerm)
}
