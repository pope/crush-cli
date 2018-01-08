package main

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"

	humanize "github.com/dustin/go-humanize"
)

var recompressCommand = ""

func recompress(ctx context.Context, src string) (err error) {
	srcFi, err := os.Stat(src)
	if err != nil {
		return
	}

	tmp, err := ioutil.TempFile(filepath.Dir(src), "recompress")
	if err != nil {
		return
	}
	defer func() {
		// If err is not nil, then it means there was some problem processing
		// things. So we should remove the tmp file.
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	if err = tmp.Close(); err != nil {
		return
	}

	jpegCmd := exec.CommandContext(ctx, recompressCommand, "--quality", "veryhigh", src, tmp.Name())
	if err = jpegCmd.Run(); err != nil {
		return
	}

	if err = os.Rename(tmp.Name(), src); err != nil {
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
	var exec string
	switch runtime.GOOS {
	case "windows":
		exec = "jpeg-recompress.exe"
	case "linux":
		exec = "jpeg-recompress-linux"
	case "darwin":
		exec = "jpeg-recompress-darwin"
	default:
		log.Fatalf("Unsupported OS: %q", runtime.GOOS)
	}
	recompressCommand = filepath.Join(dir, "vendor", exec)
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

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()

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
