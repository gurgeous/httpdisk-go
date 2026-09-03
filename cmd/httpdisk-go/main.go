package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gurgeous/httpdisk-go"
)

func main() {
	var cli struct {
		Dir    string `help:"Cache directory." type:"path"`
		Status bool   `help:"Show cache status."`
		URL    string `arg:"" name:"url" help:"URL to fetch or inspect."`
	}
	kong.Parse(
		&cli,
		kong.Name("httpdisk-go"),
		kong.Description("Fetch a URL through httpdisk-go."),
		kong.UsageOnError(),
	)

	if !strings.Contains(cli.URL, "://") {
		cli.URL = "https://" + cli.URL
	}
	req, err := http.NewRequest(http.MethodGet, cli.URL, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	hd := httpdisk.NewHTTPDisk(httpdisk.Options{Dir: cli.Dir})
	if cli.Status {
		err = status(hd, req)
	} else {
		err = get(hd, req)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func get(hd *httpdisk.HTTPDisk, req *http.Request) error {
	resp, err := (&http.Client{Transport: hd}).Do(req)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(os.Stdout, resp.Body)
	closeErr := resp.Body.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func status(hd *httpdisk.HTTPDisk, req *http.Request) error {
	status, err := hd.Status(req)
	if err != nil {
		return err
	}

	fmt.Printf("status for %s...\n\n", status.URL)
	fmt.Printf("key:    %s\n", status.Key)
	fmt.Printf("path:   %s\n", status.Path)
	fmt.Printf("status: %s\n", status.Status)
	if status.Age > 0 {
		fmt.Printf("age:    %s\n", status.Age.Truncate(time.Second))
	}
	return nil
}
