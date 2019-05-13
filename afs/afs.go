// Copyright © 2019 Valentin Slyusarev <va.slyusarev@gmail.com>

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/va-slyusarev/afs"
)

type assetWithError struct {
	asset *afs.Asset
	err   error
}

var (
	source, target string
	pkg, file      string
	exclude        string
	overwrite      bool
	compress       bool
)

func main() {
	flag.StringVar(&source, "src", path.Join(".", "web/asset"), "The path of the source directory.")
	flag.StringVar(&target, "tar", path.Join(".", "web"), "The target path of the generated package.")
	flag.StringVar(&pkg, "p", "web", "Name of the generated package.")
	flag.StringVar(&file, "f", "web.go", "Name of the generated file.")
	flag.StringVar(&exclude, "exclude", ".*.*", "Match pattern for exclude files.")
	flag.BoolVar(&overwrite, "rw", false, "Rewrite target file if it already exists.")
	flag.BoolVar(&compress, "z", true, "Use zlib compression.")
	flag.Parse()

	checkFlags()

	fmt.Printf("afs: the beginning of the generation process\n")

	assets, ok := build(walk(source))
	if !ok {
		exitWithError(fmt.Errorf("asf: exit with errors"))
	}

	if len(assets) == 0 {
		exitWithError(fmt.Errorf("asf: no files from source dir"))
	}

	if err := generate(assets); err != nil {
		exitWithError(fmt.Errorf("asf: generation error: %v", err))
	}
	fmt.Printf("afs: the generation was successful and created file: %q\n", target)
}

func checkFlags() {
	source, err := filepath.Abs(source)
	if err != nil {
		exitWithError(fmt.Errorf("afs: broken source path %q: %v", source, err))
	}

	s, err := os.Stat(source)
	if err != nil {
		exitWithError(fmt.Errorf("afs: broken source path: %v", err))
	}

	if !s.IsDir() {
		exitWithError(fmt.Errorf("afs: broken source path %q: is not dir", source))
	}

	target, err = filepath.Abs(target)
	if err != nil {
		exitWithError(fmt.Errorf("afs: broken target path %q: %v", target, err))
	}

	if _, err := os.Stat(target); err != nil {
		exitWithError(fmt.Errorf("afs: broken target: %v", err))
	}

	target = filepath.Join(target, file)

	t, err := os.Stat(target)
	if (err != nil && !os.IsNotExist(err)) || (err == nil && t.IsDir()) {
		exitWithError(fmt.Errorf("afs: broken target file name %q: %v", target, err))
	}

	if t != nil && !overwrite {
		exitWithError(fmt.Errorf("afs: file %q already exists, please, use -rw flag to rewrite", target))
	}

	if t != nil && overwrite {
		if err := os.Remove(target); err != nil {
			exitWithError(fmt.Errorf("afs: file %q could not be deleted: %v", target, err))
		}

		fmt.Printf("afs: the existing file %q was deleted and will be created in the process\n", target)
	}
}

func walk(root string) <-chan *assetWithError {

	ch := make(chan *assetWithError)
	go func() {
		var wg sync.WaitGroup
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !info.Mode().IsRegular() {
				return nil
			}
			matched, _ := filepath.Match(exclude, info.Name())
			if matched {
				fmt.Printf("afs: file %q skipping by exclude pattern %q\n", info.Name(), exclude)
				return nil
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				asset, err := makeAsset(path)
				ch <- &assetWithError{asset: asset, err: err}
			}()

			select {
			default:
				return nil
			}
		})

		go func() {
			wg.Wait()
			close(ch)
		}()
	}()

	return ch
}

func build(ch <-chan *assetWithError) (map[string]*afs.Asset, bool) {
	ok := true
	assets := make(map[string]*afs.Asset)
	for c := range ch {
		if c.err != nil {
			fmt.Printf("afs: %v\n", c.err)
			ok = false
			continue
		}
		assets[c.asset.AName] = c.asset
	}
	return assets, ok
}

func generate(assets map[string]*afs.Asset) error {
	type tplData struct {
		Package string
		Now     time.Time
		Assets  []*afs.Asset
	}
	td := &tplData{
		Package: pkg,
		Now:     time.Now(),
	}
	var sorted []string
	for name := range assets {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	for _, name := range sorted {
		if asset, ok := assets[name]; ok {
			td.Assets = append(td.Assets, asset)
		}
	}

	t := template.Must(template.New("afs").Parse(tpl))
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	return t.Execute(f, td)
}

func makeAsset(path string) (*afs.Asset, error) {

	// Read
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error read file %q: %v", path, err)
	}

	var b64 string

	// zlib
	if compress {
		z := bytes.NewBuffer([]byte{})
		w := zlib.NewWriter(z)
		_, err = w.Write(data)
		if err != nil {
			return nil, fmt.Errorf("error zlib file %q: %v", path, err)
		}
		_ = w.Close()

		b64 = base64.StdEncoding.EncodeToString(z.Bytes())
	} else {
		b64 = base64.StdEncoding.EncodeToString(data)
	}

	return &afs.Asset{
		AName:  filepath.Join("/", strings.TrimPrefix(path, source)),
		Base64: b64,
		NoZLib: !compress,
	}, nil
}

func exitWithError(err error) {
	fmt.Printf("%v\n", err)
	flag.Usage()
	os.Exit(1)
}

const tpl = `// CODE GENERATED AT {{ .Now.Format "2006/01/02 15:04:05" }} BY AFS. NOT EDIT.
//
// AFS contains the following assets:
{{- range $index, $asset := .Assets }}
// {{ $index }}) {{ $asset.AName }}
{{- end }}
//
package {{ .Package }}

import "github.com/va-slyusarev/afs"

func init() {

{{- range $index, $asset := .Assets }}
	f{{ $index }} := &afs.Asset{
		AName:  "{{ $asset.AName }}",
		Base64: "{{ $asset.Base64}}",
		{{- if $asset.NoZLib }}
		NoZLib: true,
		{{- end }}
	}
{{- end }}

	afs.Register({{ range $index, $asset := .Assets }}f{{ $index }}, {{ end }})
}
`
