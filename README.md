![badge](https://github.com/gurgeous/httpdisk-go/workflows/Test/badge.svg)

### Overview

`httpdisk` will cache http responses on disk. Several of these already exist (see below) but this one is a bit different. The priority for `httpdisk` is to always cache on disk. It is not RFC compliant. It caches GET, POST and everything else. httpdisk is useful for crawling projects, to aggressively avoid extra http requests.

This is a port of the [httpdisk](https://github.com/gurgeous/httpdisk) ruby gem. The cache key, the directory layout and the file format all match the gem, so both can share a cache directory (see [Ruby Compatibility](#ruby-compatibility)).

### Usage

Just plug httpdisk into an http.Client:

```go
hd := httpdisk.NewHTTPDisk(httpdisk.Options{})
client := http.Client{Transport: hd}
resp, err := client.Get("http://google.com")
...
```

Responses will be cached in `~/httpdisk`. The cache key is the md5 sum of the HTTP method, the normalized URL, and the request body. The path will be of the form `~/httpdisk/google.com/0e3/7f96800a55958fa6029283c78f672`. Responses never expire by default and are never deleted by httpdisk. They will last forever and grow unbounded until manually deleted.

Note that HTTP headers are NOT used to calculate the cache key. This can be unintuitive for crawling projects that involve cookies or session state.

### Options

| Option | Description |
| --- | --- |
| `Dir` | location for the disk cache, defaults to `~/httpdisk` |
| `Compress` | if true, gzip cache files as we write them. Defaults to false. We always read both compressed and plain files, so this can be flipped at any time |
| `Expires` | cached responses older than this are refetched, zero means never expire |
| `Force` | don't read anything from cache (but still write) |
| `ForceErrors` | don't read errors from cache (but still write) |
| `IgnoreParams` | query/form params to ignore when calculating cache keys, defaults to `DefaultIgnoreParams`. Use an empty (non-nil) slice to disable |
| `Logger` | optional `*log.Logger` for hit/miss logging |

Params like `api_key` or `timestamp` vary from request to request and would otherwise fragment the cache, so they are dropped from the cache key by default. See `DefaultIgnoreParams`.

### Debugging

Set `HTTPDISK_DEBUG` to log the cache status of every request to stderr:

```sh
$ HTTPDISK_DEBUG=1 ./crawler
httpdisk: GET http://google.com/ (miss)
httpdisk: GET http://google.com/ (hit)
```

`Options.Logger` does the same thing, and takes precedence. `HTTPDisk.Status()` reports the status of a single request without hitting the network:

```go
status, err := hd.Status(req)
// status.Status is one of miss, hit, error, stale or force
// status.Key, status.Digest and status.Path show how the key was calculated
```

### Aggressive Caching

httpdisk caches all responses, including POSTs, 500s and other HTTP errors. Response headers that typically control caching are completely ignored. We also cache many network errors like connection refused, timeout, ssl error and so on. Those are stored as responses with HTTP status 999 and returned to the caller as errors with `(cached)` appended. Unrecognized errors are not cached (see `cacheableErrors`).

### Ruby Compatibility

Cache keys, digests, the directory layout and the payload format all match the ruby gem, so `httpdisk --status` and `httpdisk-grep` work against caches written by this library.

Compression interoperates in both directions. Like the gem, we sniff the gzip magic bytes on read, so a cache directory can hold a mix of compressed and plain files. The only difference is the default: the gem compresses unless you pass `compress: false`, while we write plain files unless you set `Compress: true`.

### Also See

Here are some other excellent caching libraries that you might want to check out. These generally act like traditional HTTP caches:

- [https://github.com/bxcodec/httpcache](https://github.com/bxcodec/httpcache)
- [https://github.com/gregjones/httpcache](https://github.com/gregjones/httpcache)
