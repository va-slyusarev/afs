# Assets File System (AFS)
Another implementation of a virtual file system based on assets embedded in your code.
The implementation uses *zlib* compression and *base64* encoding.

## Install

```sh
go get github.com/va-slyusarev/afs...
```

## Use case
1. Add assets to the catalog;
2. Run `cmd/afs` utility by adjusting the appropriate parameters;
3. Add generated class to your code and use.

```go
package main

import (
	"log"
	"net/http"

	"github.com/va-slyusarev/afs"
	_ "your/path/which/was/generated" // init assets for afs
)

func main() {
	if fs, err := afs.GetAFS(); err == nil {
		log.Fatal(http.ListenAndServe(":8080", http.FileServer(fs)))
	}
}
```


See `_example`
