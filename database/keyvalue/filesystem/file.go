package filesystem

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"time"
)

type Filesystem struct {
	path string
}

var mux = sync.Mutex{}

func New(dsn string, valueTypes []string) (*Filesystem, error) {
	directories := []string{dsn}
	for _, dir := range valueTypes {
		directories = append(directories, path.Join(dsn, dir))
	}

	for _, directory := range directories {
		if !dirExists(directory) {
			err := os.Mkdir(directory, 0700)
			if err != nil {
				return nil, err
			}
		}
	}

	return &Filesystem{path: dsn}, nil
}

func (fs *Filesystem) SetValue(name, valueType string, metadata interface{}) error {
	return fs.writeValue(name, valueType, metadata, false)
}

func (fs *Filesystem) UpdateValue(name, valueType string, metadata interface{}) error {
	return fs.writeValue(name, valueType, metadata, true)
}

func (fs *Filesystem) writeValue(name, valueType string, metadata interface{}, update bool) error {
	mux.Lock()
	defer mux.Unlock()

	filename := path.Join(fs.path, valueType, name)

	flag := os.O_EXCL | os.O_RDWR | os.O_CREATE
	if update {
		flag = os.O_RDWR | os.O_CREATE
	}
	f, err := os.OpenFile(filename, flag, 0600)
	if err == nil {
		defer f.Close()
	} else {
		return err
	}

	j, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = f.Write(j)
	if err != nil {
		return err
	}

	return nil
}

func (fs *Filesystem) GetExpiredValues(valueType string, t *time.Time) ([]string, error) {
	expired := make([]string, 0)

	dirname := path.Join(fs.path, valueType)
	files, err := ioutil.ReadDir(dirname)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		var record struct{ CreatedOn int64 }
		_, err := fs.GetValue(file.Name(), valueType, &record)
		if err != nil {
			return expired, err
		}
		if record.CreatedOn < t.UnixNano() {
			expired = append(expired, file.Name())
		}
	}

	return expired, nil
}

func (fs *Filesystem) DeleteValue(name, valueType string) error {
	mux.Lock()
	defer mux.Unlock()

	filename := path.Join(fs.path, valueType, name)
	if fileExists(filename) {
		err := os.Remove(filename)
		if err != nil {
			return err
		}
	}

	return nil
}

func (fs *Filesystem) ListValues(valueType string) ([]string, error) {
	files, err := ioutil.ReadDir(path.Join(fs.path, valueType))
	if err != nil {
		return nil, err
	}

	names := make([]string, len(files))
	for i, file := range files {
		names[i] = file.Name()
	}

	return names, nil
}

func (fs *Filesystem) GetValue(name, valueType string, metadata interface{}) (string, error) {
	mux.Lock()
	defer mux.Unlock()

	filename := path.Join(fs.path, valueType, name)
	f, err := os.Open(filename)
	if err == nil {
		defer f.Close()
	} else if os.IsNotExist(err) {
		return "", nil
	} else {
		return "", err
	}

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal([]byte(data), &metadata)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func dirExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}
