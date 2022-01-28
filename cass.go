package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path"
	"time"
)

type cass struct {
	port      string
	fileStore string
	urlBase   string
	tempDir   string
	userAgent string
}

func moveFile(sourcePath, destPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("Couldn't open source file: %s", err)
	}
	outputFile, err := os.Create(destPath)
	if err != nil {
		inputFile.Close()
		return fmt.Errorf("Couldn't open dest file: %s", err)
	}
	defer outputFile.Close()
	_, err = io.Copy(outputFile, inputFile)
	inputFile.Close()
	if err != nil {
		return fmt.Errorf("Writing to output file failed: %s", err)
	}
	// The copy was successful, so now delete the original file
	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("Failed removing original file: %s", err)
	}
	return nil
}

func (c *cass) httpGet(link string) (io.ReadCloser, error) {
	cj, err := cookiejar.New(nil)
	tr := &http.Transport{
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	}
	client := &http.Client{
		Transport: tr,
		Jar:       cj,
	}

	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (c *cass) saver(r io.Reader, filename string) (string, error) {
	ext := path.Ext(filename)

	temp, err := ioutil.TempFile(c.tempDir, "cass_")
	if err != nil {
		return "", err
	}

	hash := sha1.New()

	multi := io.MultiWriter(hash, temp)

	_, err = io.Copy(multi, r)
	if err != nil {
		return "", err
	}

	destinationFilename := fmt.Sprintf("%x%s", hash.Sum(nil), ext)
	destination := path.Join(c.fileStore, destinationFilename)
	old := temp.Name()
	temp.Close()
	err = os.Rename(old, destination)
	if err != nil {
		err = moveFile(old, destination)
		if err != nil {
			return "", err
		}
	}
	_ = os.Chmod(destination, 0644)

	return destinationFilename, nil
}

func (c *cass) linkReceiver(w http.ResponseWriter, r *http.Request) {
	link := r.FormValue("url")
	linkFilename := r.FormValue("filename")

	fileStream, err := c.httpGet(link)
	defer fileStream.Close()
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

	filename, err := c.saver(fileStream, linkFilename)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
	fmt.Fprintf(w, "%s%s", c.urlBase, filename)
}

func (c *cass) fileReceiver(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	defer file.Close()
	if err != nil {
		http.Error(w, err.Error(), 500)
	}

	filename, err := c.saver(file, header.Filename)
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
	fmt.Fprintf(w, "%s%s", c.urlBase, filename)
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8000", "port to listen on")
	fileStore := flag.String("file-store", "/srv/www/arachnist.is-a.cat/c", "directory to store files in")
	urlBase := flag.String("url-base", "https://arachnist.is-a.cat/c/", "url prefix")
	tempDir := flag.String("tmp-dir", "./tmp", "temporary files directory")
	userAgent := flag.String("useragent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.212 Safari/537.36", "user-agent string for link downloads")

	flag.Parse()

	c := cass{
		fileStore: *fileStore,
		urlBase:   *urlBase,
		tempDir:   *tempDir,
		userAgent: *userAgent,
	}

	http.HandleFunc("/up", c.fileReceiver)
	http.HandleFunc("/down", c.linkReceiver)
	log.Fatalln(http.ListenAndServe(*listen, nil))
}
