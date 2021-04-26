package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/usnistgov/ndn-dpdk/ndn"
	"github.com/usnistgov/ndn-dpdk/ndn/an"
	"github.com/usnistgov/ndn-dpdk/ndn/segmented"
	"go.uber.org/zap"
)

func handleV2(w http.ResponseWriter, req *http.Request) {
	logEntry := logger.With(zap.String("method", req.Method), zap.String("uri", req.URL.String()))
	req.URL.Scheme = upstreamScheme
	req.URL.Host = upstreamHost

	if name := parseBlobRequest(req.URL); name != nil && req.Method == http.MethodGet {
		if e := proxyToNDN(logEntry, w, req, name); e != nil {
			redirectToHTTP(logEntry, w, req)
		}
		return
	}
	proxyToHTTP(logEntry, w, req)
}

func parseBlobRequest(url *url.URL) ndn.Name {
	comps := strings.Split(url.Path, "/")
	if len(comps) != 5 || comps[3] != "blobs" || !strings.HasPrefix(comps[4], "sha256:") {
		return nil
	}

	blobDigest, e := hex.DecodeString(comps[4][7:])
	if e != nil || len(blobDigest) != sha256.Size {
		return nil
	}

	name := append(ndn.Name{}, upstreamPrefix...)
	name = append(name,
		ndn.MakeNameComponent(an.TtGenericNameComponent, []byte(comps[2])),
		ndn.MakeNameComponent(an.TtGenericNameComponent, blobDigest),
	)
	return name
}

var ndnThrottle sync.Mutex

func proxyToNDN(logEntry *zap.Logger, w http.ResponseWriter, req *http.Request, name ndn.Name) error {
	logEntry = logEntry.With(zap.Stringer("name", name))
	logEntry.Debug("fetch queued")
	t0 := time.Now()
	ndnThrottle.Lock()
	defer ndnThrottle.Unlock()
	logEntry.Debug("fetch start", zap.Duration("waited", time.Since(t0)))

	timeout, cancel := context.WithTimeout(req.Context(), ndnFetchTimeout)
	defer cancel()

	fetcher := segmented.Fetch(name, ndnFetchOptions)
	t0 = time.Now()
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		historyCounts := make([]int, 3)
		for i := range historyCounts {
			historyCounts[i] = -1
		}

		for t1 := range ticker.C {
			cnt, total := fetcher.Count(), fetcher.EstimatedTotal()
			logEntry.Debug("fetch progress",
				zap.Duration("elapsed", t1.Sub(t0)),
				zap.Int("count", cnt),
				zap.Int("estimated-total", total),
				zap.Float64("ratio", float64(cnt)/float64(total)),
			)

			if cnt == historyCounts[0] {
				logEntry.Warn("fetch no progress, canceling")
				cancel()
			}
			copy(historyCounts, historyCounts[1:])
			historyCounts[len(historyCounts)-1] = cnt
		}
	}()

	blob, e := fetcher.Payload(timeout)
	ticker.Stop()
	if e != nil {
		logEntry.Warn("fetch error", zap.Error(e))
		return e
	}
	logEntry.Debug("fetch success",
		zap.Duration("duration", time.Since(t0)),
		zap.Int("size", len(blob)),
	)

	digest := sha256.Sum256(blob)
	if subtle.ConstantTimeCompare(name.Get(-1).Value, digest[:]) != 1 {
		logEntry.Warn("bad digest")
		return errors.New("bad digest")
	}

	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(blob)
	return nil
}

func redirectToHTTP(logEntry *zap.Logger, w http.ResponseWriter, req *http.Request) {
	logEntry.Debug("redirect", zap.Stringer("dest", req.URL))
	http.Redirect(w, req, req.URL.String(), http.StatusTemporaryRedirect)
}

func proxyToHTTP(logEntry *zap.Logger, w http.ResponseWriter, req *http.Request) {
	logEntry = logEntry.With(zap.Stringer("upstream", req.URL))

	uReq, e := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
	if e != nil {
		logEntry.Warn("proxy error", zap.Error(e))
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	uReq.Header = req.Header

	resp, e := http.DefaultClient.Do(uReq)
	if e != nil {
		logEntry.Warn("proxy error", zap.Error(e))
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	logEntry.Debug("proxied",
		zap.Int("status", resp.StatusCode),
		zap.String("length", resp.Header.Get("Content-Length")),
	)

	for header, headerSlice := range resp.Header {
		for _, headerValue := range headerSlice {
			w.Header().Add(header, headerValue)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
