package main

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	humanize "github.com/dustin/go-humanize"
)

var recompressCommand = ""

func recompress(ctx context.Context, src string) error {
	srcFi, err := os.Stat(src)
	if err != nil {
		return err
	}

	tmp, err := ioutil.TempFile("", "recompress")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Close(); err != nil {
		return err
	}

	jpegCmd := exec.CommandContext(ctx, recompressCommand, "--quality", "veryhigh", src, tmp.Name())
	if err := jpegCmd.Run(); err != nil {
		return err
	}

	// No ctx here because I don't want to interrupt the copy.
	cpCmd := exec.Command("cmd", "/C", "copy", "/y", tmp.Name(), src)
	if err := cpCmd.Run(); err != nil {
		return err
	}

	dstFi, err := os.Stat(src)
	if err != nil {
		return err
	}

	diff := srcFi.Size() - dstFi.Size()
	pct := (float64(diff) / float64(srcFi.Size())) * 100.0
	log.Printf("%s: savings of %s (decreased by %0.2f%%)\n", src, humanize.Bytes(uint64(diff)), pct)
	return nil
}

// filepath.Glob wasn't working for OneDrive stuff.
// See https://github.com/golang/go/issues/22579.
func findJPEGs(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	res := make([]string, 0)
	for _, f := range files {
		ext := filepath.Ext(f.Name())
		if strings.EqualFold(ext, ".JPG") {
			res = append(res, filepath.Join(dir, f.Name()))
		}
	}
	return res, nil
}

func initCompressCommandPath() {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	recompressCommand = filepath.Join(dir, "jpeg-recompress.exe")
}

func processArgs(args []string) ([]string, error) {
	if len(args) == 0 {
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		return findJPEGs(dir)
	}
	return args, nil
}

func main() {
	var err error

	initCompressCommandPath()

	jpegs, err := processArgs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	if len(jpegs) == 0 {
		log.Println("No JPEGS to recompress.")
		return
	}

	numWorkers := runtime.NumCPU()
	sem := make(chan int, numWorkers)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(len(jpegs))

	for _, jpeg := range jpegs {
		go func(src string) {
			defer wg.Done()

			select {
			case sem <- 1:
			case <-ctx.Done():
				return
			}

			err := recompress(ctx, src)
			if err != nil {
				log.Printf("E: [%s] %q", src, err)
				cancel()
			}

			<-sem
		}(jpeg)
	}
	wg.Wait()
}
