// Copyright © 2019 Valentin Slyusarev <va.slyusarev@gmail.com>

//go:generate afs -rw
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/va-slyusarev/afs"
	_ "github.com/va-slyusarev/afs/_example/web" // init assets for afs
)

var port = flag.Int("port", 8090, "Server port.")

func main() {
	flag.Parse()

	fs, err := afs.GetAFS()

	if err != nil {
		log.Fatalf("%v", err)
	}

	custom := &afs.Asset{AName: "/custom.md", Base64: "IyBIaSBjdXN0b20gYXNzZXQh", NoZLib: true}

	if err := fs.Add(custom); err != nil {
		log.Fatalf("%v", err)
	}

	log.Printf("afs is ready!\n%v", fs)

	go func() {
		select {
		case <-time.After(30 * time.Second):
			if err := fs.ExecTemplate([]string{"index.html"}, map[string]bool{"use": true}); err != nil {
				log.Fatalf("%v", err)
			}
			log.Printf("template exec successful! See http://localhost:%d\n", *port)
		}
	}()

	log.Printf("server start on http://localhost:%d\n", *port)
	http.Handle("/", http.FileServer(fs))
	_ = http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}
