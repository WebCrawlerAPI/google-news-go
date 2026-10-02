package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	googlenews "github.com/WebCrawlerAPI/google-news-go"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: google-news-go <news.google.com URL>")
	}
	res, err := googlenews.Resolve(context.Background(), os.Args[1])
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}
