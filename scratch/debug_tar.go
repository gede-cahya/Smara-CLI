package scratch

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func mainDebugTar() {
	f, err := os.Open("test.tar.gz")
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		fmt.Printf("Error gzip reader: %v\n", err)
		return
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	found := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error tr.Next: %v\n", err)
			return
		}

		fmt.Printf("Found file: %s (Type: %v)\n", header.Name, header.Typeflag)
		
		if header.Typeflag == tar.TypeReg {
			base := filepath.Base(header.Name)
			fmt.Printf("  Base: %s\n", base)
			if base == "smara" || base == "smara.exe" || strings.HasPrefix(base, "smara-") {
				fmt.Printf("  MATCHED!\n")
				found = true
			}
		}
	}

	if !found {
		fmt.Println("No binary found in tarball.")
	}
}
