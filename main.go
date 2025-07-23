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

var TTL time.Duration = 0
var CleanUpPeriod time.Duration = 0

const (
	XCacheMiss = "MISS"
	XCacheHit  = "HIT"
)

const (
	ReadTimeoutAmount   = 10
	WriteTimeoutAmount  = 10
	FlushIntervalAmount = 10
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
		FlushInterval: FlushIntervalAmount * time.Millisecond,
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

	cup := getCleanUpPeriod()
	c.startCleanupWorker(cup)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			c.mu.RLock()
			d, ok := c.data[r.RequestURI]
			c.mu.RUnlock()

			if ok && !isCacheStale(d.age, c.ttl) {
				writeToResponseCacheHit(w, d)

				return
			}

			handleMissedCache(rp, c)
		}

		rp.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  ReadTimeoutAmount * time.Second,
		WriteTimeout: WriteTimeoutAmount * time.Second,
	}
	defer func(srv *http.Server) {
		if err := srv.Close(); err != nil {
			log.Fatal("Cannot close server")
		}
	}(srv)

	log.Printf("Reverse-proxy listening on %s", srv.Addr)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

	return nil
}

func handleMissedCache(rp *httputil.ReverseProxy, c *cache) {
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

func writeToResponseCacheHit(w http.ResponseWriter, d cacheData) {
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
	}
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
	if 0 != TTL {
		return TTL
	}

	if err := godotenv.Load(); err != nil {
		log.Fatalf("error loading .env file %s", err)
	}

	hours, err := strconv.Atoi(os.Getenv("TTL"))

	if nil != err {
		log.Fatalf("cannot convert ttl to int %s", err)
	}

	TTL = time.Duration(hours) * time.Hour

	return TTL
}

func getCleanUpPeriod() time.Duration {
	if 0 != CleanUpPeriod {
		return CleanUpPeriod
	}

	if err := godotenv.Load(); err != nil {
		log.Fatalf("error loading .env file %s", err)
	}

	hours, err := strconv.Atoi(os.Getenv("CLEAN_UP_PERIOD"))

	if nil != err {
		log.Fatalf("cannot convert ttl to int %s", err)
	}

	CleanUpPeriod = time.Duration(hours) * time.Hour

	return CleanUpPeriod
}

func isCacheStale(a time.Time, ttl time.Duration) bool {
	return time.Now().After(a.Add(ttl))
}

func (c *cache) startCleanupWorker(i time.Duration) {
	go func() {
		ticker := time.NewTicker(i)
		ttl := getTTL()
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.cleanup(ttl)
			}
		}
	}()
}

func (c *cache) cleanup(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, d := range c.data {
		if isCacheStale(d.age, ttl) {
			delete(c.data, key)
			log.Printf("deleted cache with key: %s", key)
		}
	}

	log.Println("cache cleanup completed")
}
