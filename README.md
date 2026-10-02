# google-news-go

Resolve `news.google.com` article links to the real publisher URL. No dependencies, no browser.

## Install

```sh
go get github.com/WebCrawlerAPI/google-news-go
```

CLI:

```sh
go install github.com/WebCrawlerAPI/google-news-go/cmd/google-news-go@latest
google-news-go 'https://news.google.com/read/ARTICLE_ID'
```

## Usage

```go
res, err := googlenews.Resolve(ctx, "https://news.google.com/read/ARTICLE_ID")
if err != nil {
	log.Fatal(err)
}
fmt.Println(res.URL)
```

Uses an undocumented Google endpoint, so it can break if Google changes it. Cache results and back off on 429.

## License

MIT
