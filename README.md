![badge](https://github.com/gurgeous/httpdisk-go/actions/workflows/ci.yml/badge.svg)

### Overview

`httpdisk` will cache http responses on disk. Several of these already exist (see below) but this one is a bit different. The priority for `httpdisk` is to always cache on disk. It is not RFC compliant. It caches GET, POST and everything else. httpdisk is useful for crawling projects, to aggressively avoid extra http requests.

### Usage

Just plug httpdisk into an http.Client:

```go
hd := httpdisk.NewHTTPDisk(httpdisk.Options{})
client := http.Client{Transport: hd}
resp, err := client.Get("https://google.com")
...
```

Responses will be cached in `~/httpdisk-go`. The cache key is the SHA-256 sum of the HTTP method, the normalized URL, and the request body. The path will be of the form `~/httpdisk-go/google.com/824/765eecd29fc4e76a36c461f65a4e7fc670f4275161149f0cc9d63ffb45b48`. Responses never expire by default and are never deleted by httpdisk. They will last forever and grow unbounded until manually deleted.

Note that HTTP headers are NOT used to calculate the cache key. This can be unintuitive for crawling projects that involve cookies or session state.

### Options

| Option         | Description                                                                                                                                       |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Dir`          | location for the disk cache, defaults to `~/httpdisk-go`                                                                                          |
| `Compress`     | if true, gzip cache files as we write them. Defaults to false. We always read both compressed and plain files, so this can be flipped at any time |
| `Expires`      | cached responses older than this are refetched, zero means never expire                                                                           |
| `Force`        | don't read anything from cache (but still write)                                                                                                  |
| `ForceErrors`  | don't read errors from cache (but still write)                                                                                                    |
| `IgnoreParams` | query/form params to ignore when calculating cache keys, defaults to none                                                                         |
| `Logger`       | optional `*log.Logger` for hit/miss logging                                                                                                       |

Params like timestamps can vary from request to request and fragment the cache. Use `DefaultIgnoreParams` as a starting point when that is safe for your application:

```go
hd := httpdisk.NewHTTPDisk(httpdisk.Options{
    IgnoreParams: httpdisk.DefaultIgnoreParams,
})
```

This list includes authentication parameters such as `api_key` and `token`. Remove those entries when they identify different users or permissions.

### Debugging

Use `HTTPDISK_DEBUG=1` to log requests to stderr:

```sh
$ HTTPDISK_DEBUG=1 ./crawler
httpdisk: GET https://google.com/ (miss)
httpdisk: GET https://google.com/ (hit)
```

`Options.Logger` does the same thing, and takes precedence. `HTTPDisk.Status()` reports the status of a single request without hitting the network:

```go
status, err := hd.Status(req)
// status.Status is one of miss, hit, error, stale or force
// status.Key, status.Digest and status.Path show how the key was calculated
```

### Aggressive Caching

httpdisk caches all responses, including POSTs, 500s and other HTTP errors. Response headers that typically control caching are completely ignored. We also cache many network errors like connection refused, timeout, ssl error and so on. Those are stored as responses with HTTP status 999 and returned to the caller as errors with `(cached)` appended. Unrecognized errors are not cached (see `cacheableErrors`).

### Also See

Here are some other excellent caching libraries that you might want to check out. These generally act like traditional HTTP caches:

- [https://github.com/bxcodec/httpcache](https://github.com/bxcodec/httpcache)
- [https://github.com/gregjones/httpcache](https://github.com/gregjones/httpcache)

### Changelog

#### Sep 2026

- Modernized, removed CLIs
- Added gzip and other options
- Moved cache to `~/httpdisk-go`
- Switched cache keys to sha (legacy was md5)
