// Resizes all the JPEGs in a directory to a list
// of sizes and stores them in a new directory.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
)

var fromPath, toPath, sizesString string

func init() {
	fromPath = "."
	flag.StringVar(&fromPath, "f", fromPath, "directory of original images")

	toPath = "."
	flag.StringVar(&toPath, "t", toPath, "directory to store resized images")

	flag.StringVar(&sizesString, "s", "", "comma-separated list of sizes")
}

type imageData struct {
	img  image.Image
	name string
}

func main() {
	flag.Parse()

	try(
		validateDirectory(fromPath, true),
		validateDirectory(toPath, false),
		os.MkdirAll(toPath, 0755),
	)

	sizes, err := parseSizes(sizesString)
	if err != nil {
		log.Fatal("can't parse sizes:", err)
	}
	if len(sizes) <= 0 {
		log.Fatal("no sizes specified")
	}

	files, err := filepath.Glob(filepath.Join(fromPath, "*.jpg"))
	if err != nil {
		log.Fatal("can't get image filenames:", err)
	}
	if len(files) <= 0 {
		log.Fatal("no images to resize")
	}

	images := make(chan *imageData, runtime.GOMAXPROCS(0))
	go readImages(files, images)

	finished := make(chan int, runtime.GOMAXPROCS(0))
	go resizeImages(images, sizes, toPath, finished)
	<-finished
}

func try(errs ...error) {
	for _, err := range errs {
		if err != nil {
			log.Fatal(err)
		}
	}
}

func validateDirectory(path string, mustExist bool) error {
	s, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if mustExist {
				return fmt.Errorf("directory %v doesn't exist", path)
			}
			return nil
		}
		return err
	}
	if !s.IsDir() {
		return fmt.Errorf("%v is not a directory", path)
	}
	return nil
}

func parseSizes(s string) ([]int, error) {
	isComma := func(r rune) bool {
		return r == ','
	}

	var sizes []int
	for _, t := range strings.FieldsFunc(s, isComma) {
		i, err := strconv.ParseInt(t, 10, 0)
		switch {
		case err != nil && err.(*strconv.NumError).Err == strconv.ErrSyntax:
			return nil, fmt.Errorf("%s is not a valid size", s)
		case err != nil && err.(*strconv.NumError).Err == strconv.ErrRange:
			return nil, fmt.Errorf("size %s is out of the valid range", s)
		case i < 0:
			return nil, fmt.Errorf("size %d is less than zero", i)
		}

		sizes = append(sizes, int(i))
	}

	return sizes, nil
}

func readImages(paths []string, c chan *imageData) {
	finished := make(chan int, runtime.GOMAXPROCS(0))
	for _, path := range paths {
		go readImage(path, c, finished)
	}

	for i := 0; i < len(paths); i++ {
		<-finished
	}
	close(c)
}

func readImage(path string, images chan *imageData, finished chan int) {
	log.Println("reading", path)
	defer func() { finished <- 1 }()
	file, err := os.Open(path)
	if err != nil {
		log.Println(err)
		return
	}
	defer file.Close()

	img, err := jpeg.Decode(file)
	if err != nil {
		log.Println(err)
		return
	}

	images <- &imageData{
		img,
		strings.Replace(filepath.Base(path), filepath.Ext(path), "", -1),
	}
}

func resizeImages(images chan *imageData, sizes []int, toPath string, allFinished chan int) {
	finished := make(chan int, runtime.GOMAXPROCS(0))
	count := 0
	for i := range images {
		for _, s := range sizes {
			count++
			go resizeImage(i, s, toPath, finished)
		}
	}

	for i := 0; i < count; i++ {
		<-finished
	}
	allFinished <- 1
}

func resizeImage(img *imageData, size int, basePath string, finished chan int) {
	defer func() { finished <- 1 }()
	path := filepath.Join(basePath, fmt.Sprintf("%s_%d.jpg", img.name, size))
	log.Println("creating", path)
	file, err := os.Create(path)
	if err != nil {
		log.Println(err)
		return
	}
	defer file.Close()

	resized := imaging.Fit(img.img, size, size, imaging.Lanczos)

	if err := jpeg.Encode(file, resized, nil); err != nil {
		log.Println(err)
	}
}
