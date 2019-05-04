// Copyright © 2019 Valentin Slyusarev <va.slyusarev@gmail.com>

package afs

// Assets File System (AFS).
// Implement a virtual file system based on assets embedded in code.

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"
)

// Asset the primary type that defines the asset. This type implements the
// necessary interfaces: http.File, os.FileInfo.
type Asset struct {
	AName  string // name of asset.
	Base64 string // content of asset base64 encoding.
	NoZLib bool   // The label of compression of content zlib.

	isDir   bool
	content string
	size    int64

	io.ReadSeeker
}

// AFS Assets File System. This type implement http.FileSystem interface.
type AFS struct {
	mu     sync.Mutex
	assets map[string]*Asset
}

var assets []*Asset
var assetFileSystem *AFS

var once sync.Once
var onceAFS = func() {
	assetFileSystem = new(AFS)
}

// Register registering assets for future inclusion in the AFS.
func Register(a ...*Asset) {
	assets = append(assets, a...)
}

// GetAFS get and reload AFS.
func GetAFS() (*AFS, error) {
	once.Do(onceAFS)
	err := assetFileSystem.reload()
	return assetFileSystem, err
}

// Add new assets in AFS.
func (afs *AFS) Add(a ...*Asset) error {
	assets = append(assets, a...)
	return afs.reload()
}

// Files get all sorted files and dir name from AFS.
func (afs *AFS) Files() []string {
	var sorted []string
	for name := range afs.assets {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	return sorted
}

// Belong file or dir belong to the AFS.
func (afs *AFS) Belong(name string) bool {
	name = fileNameClean(name)
	_, ok := afs.assets[name]
	return ok
}

// ExecTemplate performs text/template considering the content asset template.
// The result of the templating is put back into the asset content.
func (afs *AFS) ExecTemplate(names []string, data interface{}) error {
	for _, name := range names {
		asset, err := afs.asset(name)
		if err != nil {
			return fmt.Errorf("afs: exec template: find asset: %v", err)
		}
		if asset.isDir {
			return fmt.Errorf("afs: exec template: asset is dir")
		}

		tpl, err := template.New(name).Parse(asset.content)
		if err != nil {
			return fmt.Errorf("afs: exec template: parse template: %v", err)
		}

		buf := bytes.NewBuffer([]byte{})
		if err = tpl.Execute(buf, data); err != nil {
			return fmt.Errorf("afs: exec template: %v", err)
		}
		asset.content = buf.String()
		asset.size = int64(buf.Len())

		// It's all. Attention! No update registered assets.
	}
	return nil
}

// String Stringer interface implementation.
// Files are displayed in sorted form.
func (afs *AFS) String() string {
	files := afs.Files()
	buf := strings.Builder{}
	total := 0
	size := int64(0)
	buf.WriteString(fmt.Sprintf("AFS contains the following assets:\n"))
	for _, name := range files {
		if a, err := afs.asset(name); err == nil {
			buf.WriteString(fmt.Sprintf("%s\n", a.AName))
			total++
			size += a.size
		}
	}
	buf.WriteString(fmt.Sprintf("Total: %d file(s) and dir(s) of size is %d bytes.\n", total, size))
	return buf.String()
}

// Get asset from AFS by name.
func (afs *AFS) asset(name string) (*Asset, error) {
	name = fileNameClean(name)
	a, ok := afs.assets[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return a, nil
}

// fileNameClean correct file name path
func fileNameClean(name string) string {
	return filepath.Clean(filepath.Join("/", name))
}

// Rebuilding the AFS.
func (afs *AFS) reload() error {
	afs.mu.Lock()
	defer afs.mu.Unlock()
	if len(assets) == 0 {
		return errors.New("afs: no fs data registered")
	}

	type assetWithError struct {
		asset *Asset
		err   error
	}

	ch := make(chan *assetWithError)

	go func() {
		var wg sync.WaitGroup
		for _, a := range assets {
			asset := a
			wg.Add(1)
			go func() {
				defer wg.Done()
				name := fileNameClean(asset.AName)

				// base64
				b64, err := base64.StdEncoding.DecodeString(asset.Base64)
				if err != nil {
					ch <- &assetWithError{err: fmt.Errorf("decode asset %s: %v", name, err)}
					return
				}
				content := string(b64)

				// zlib
				if !asset.NoZLib {
					b := bytes.NewReader(b64)
					r, err := zlib.NewReader(b)
					if err != nil {
						ch <- &assetWithError{err: fmt.Errorf("zlib asset %s: %v", name, err)}
						return

					}
					z := bytes.NewBuffer([]byte{})
					_, err = io.Copy(z, r)
					if err != nil {
						_ = r.Close()
						ch <- &assetWithError{err: fmt.Errorf("zlib read asset %s: %v", name, err)}
						return
					}
					content = z.String()
					_ = r.Close()
				}

				ch <- &assetWithError{asset: &Asset{
					AName:   name,
					isDir:   false,
					content: content,
					size:    int64(len(content)),
				}}
			}()
		}

		go func() {
			wg.Wait()
			close(ch)
		}()
	}()

	afs.assets = make(map[string]*Asset, len(assets))
	for a := range ch {
		if a.err != nil {
			return fmt.Errorf("asf: %v", a.err)
		}
		afs.assets[a.asset.AName] = a.asset
	}

	// fill dirs
	for _, a := range afs.assets {
		asset := a
		dir := path.Dir(asset.AName)
		if ok := afs.Belong(dir); !ok {
			afs.assets[dir] = &Asset{AName: dir, isDir: true, content: dir, size: int64(len(dir))}
		}
	}
	return nil
}

// Open implements http.FileSystem.
func (afs *AFS) Open(name string) (http.File, error) {
	a, err := afs.asset(name)
	if err != nil {
		return nil, fmt.Errorf("afs: open file: %v", err)
	}
	a.ReadSeeker = strings.NewReader(a.content)
	return a, nil
}

// Implements os.FileInfo.
func (a *Asset) Name() string       { return a.AName }
func (a *Asset) Sys() interface{}   { return nil }
func (a *Asset) ModTime() time.Time { return time.Time{} }
func (a *Asset) IsDir() bool        { return a.isDir }
func (a *Asset) Size() int64        { return a.size }
func (a *Asset) Mode() os.FileMode {
	if a.isDir {
		return 0755 | os.ModeDir
	}
	return 0644
}

// Implements http.File.
// The rest is implemented within io.ReadSeeker.
func (a *Asset) Close() error               { return nil }
func (a *Asset) Stat() (os.FileInfo, error) { return a, nil }
func (a *Asset) Readdir(count int) ([]os.FileInfo, error) {
	var assets []os.FileInfo
	if !a.isDir {
		return assets, nil
	}
	prefix := a.AName
	for name, asset := range assetFileSystem.assets {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
			assets = append(assets, asset)
		}
	}
	return assets, nil
}
