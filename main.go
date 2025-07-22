package main

import (
	"bytes"
	"github.com/joho/godotenv"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	XCacheMiss = "MISS"
	XCacheHit  = "HIT"
)

type cacheData struct {
	header http.Header
	body   []byte
	age    time.Time
	status int
}
type cache struct {
	mu   sync.RWMutex
	data map[string]cacheData
	ttl  time.Duration
}

func newCache(ttl time.Duration) *cache {
	return &cache{
		data: make(map[string]cacheData),
		ttl:  ttl,
	}
}

func newReverseProxy(urlName string) *httputil.ReverseProxy {
	target, err := url.Parse(urlName)

	if err != nil {
		log.Fatal("could not parse server url")
	}

	d := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		req.Header.Del("X-Forwarded-For")
	}

	return &httputil.ReverseProxy{
		FlushInterval: 10 * time.Millisecond,
		Director:      d,
	}
}

func main() {
	if err := run(); err != nil {
		log.Panic("unexpected error during runtime", err)
	}
}

func run() error {
	rp := newReverseProxy("https://dummyjson.com")
	ttl := getTTL()
	c := newCache(ttl)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			c.mu.RLock()
			d, ok := c.data[r.RequestURI]
			c.mu.RUnlock()

			if ok && !isCacheStale(d.age) {
				for k, vv := range d.header {
					for _, v := range vv {
						w.Header().Add(k, v)
					}
				}

				w.Header().Set("X-Cache", XCacheHit)
				w.WriteHeader(d.status)

				_, err := w.Write(d.body)

				if err != nil {
					log.Printf("can't write to body %s", err)

					return
				}

				return
			}

			rp.ModifyResponse = func(res *http.Response) error {
				if res.Request.Method != http.MethodGet {
					return nil
				}

				err := saveCacheData(res, c, XCacheMiss)

				if nil != err {
					log.Printf("error while saving stale cache %s", err)
				}
				return nil
			}
		}

		rp.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	defer func(srv *http.Server) {
		err := srv.Close()
		if err != nil {
			log.Fatal("Cannot close server")
		}
	}(srv)

	log.Printf("Reverse-proxy listening on %s", srv.Addr)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

	return nil
}

func saveCacheData(res *http.Response, c *cache, xCacheValue string) error {
	key := res.Request.RequestURI

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	err = res.Body.Close()
	if err != nil {
		return err
	}

	res.Body = io.NopCloser(bytes.NewReader(b))

	c.mu.Lock()
	c.data[key] = cacheData{
		header: res.Header.Clone(),
		body:   b,
		age:    time.Now(),
		status: res.StatusCode,
	}
	c.mu.Unlock()

	res.Header.Add("X-Cache", xCacheValue)
	return nil
}

func getTTL() time.Duration {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("error loading .env file %s", err)
	}

	hours, err := strconv.Atoi(os.Getenv("TTL"))

	if nil != err {
		log.Fatalf("cannot convert ttl to int %s", err)
	}

	return time.Duration(hours) * time.Hour
}

func isCacheStale(age time.Time) bool {
	ttl := getTTL()
	return time.Now().After(age.Add(ttl))
}
