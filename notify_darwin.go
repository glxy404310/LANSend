package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// The parent app reads only complete, tagged records from its child's pipe.
func notifyReceived(path string) {
	fmt.Fprintln(os.Stdout, "RECEIVED "+base64.StdEncoding.EncodeToString([]byte(filepath.Base(path))))
}
