package main

import (
	"context"
	"fmt"
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
	if !srcFi.Mode().IsRegular() {
		return fmt.Errorf("recompress: %s non-regular file - %q", src, srcFi.Mode().String())
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

	cpCmd := exec.CommandContext(ctx, "cmd", "/C", "copy", "/y", tmp.Name(), src)
	if err := cpCmd.Run(); err != nil {
		return err
	}

	dstFi, err := os.Stat(src)
	if err != nil {
		return err
	}

	diff := srcFi.Size() - dstFi.Size()
	pct := (float64(diff) / float64(srcFi.Size())) * 100.0
	fmt.Printf("%s: savings of %s (decreased by %0.2f%%)\n", src, humanize.Bytes(uint64(diff)), pct)
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
	// TODO(pope): Return the actual parameters after they have been vetted.
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

	log.Fatal(jpegs)

	errChan := make(chan error, numWorkers)
	doneChan := make(chan string)

	workChan := make(chan string)
	var activeWg sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < numWorkers; i++ {
		go func() {
			for src := range workChan {
				activeWg.Add(1)
				err := recompress(ctx, src)
				if err != nil {
					errChan <- err
				}
				activeWg.Done()
			}
		}()
	}

	go func() {
	LOOP:
		for _, src := range jpegs {
			select {
			case <-ctx.Done():
				break LOOP
			case workChan <- src:
			}
		}
		close(workChan)
		activeWg.Wait()
		doneChan <- "done"
	}()

	select {
	case err = <-errChan:
		cancel()
	case <-doneChan:
	}
	// This is currently a race conditions. But I need to get sleep now.
	activeWg.Wait()

	if err != nil {
		log.Fatal(err)
	}
	log.Print("All done!")
}
